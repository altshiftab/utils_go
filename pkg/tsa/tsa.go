// Package tsa requests RFC 3161 timestamp tokens and reads the TSTInfo inside them.
package tsa

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"time"

	"github.com/altshiftab/utils_go/pkg/cms"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

const (
	requestContentType  = "application/timestamp-query"
	responseContentType = "application/timestamp-reply"
	nonceBits           = 64
	// maxResponseSize is far past any token, with its certificates, a service sends.
	maxResponseSize = 1 << 20
)

// ErrBadResponse is a timestamping service that refused, answered something unreadable, or answered with a token that is not for this request or not from a trusted authority; it is the service's failure, not the caller's input.
var ErrBadResponse = errors.New("bad timestamping service response")

// Timestamper returns a timestamp token, a CMS ContentInfo, over the SHA-256 digest given.
type Timestamper interface {
	Timestamp(ctx context.Context, digest []byte) ([]byte, error)
}

type messageImprint struct {
	HashAlgorithm pkix.AlgorithmIdentifier
	HashedMessage []byte
}

type timeStampRequest struct {
	Version        int
	MessageImprint messageImprint
	Nonce          *big.Int `asn1:"optional"`
	CertReq        bool     `asn1:"optional"`
}

type pkiStatusInfo struct {
	Status       int
	StatusString asn1.RawValue  `asn1:"optional"`
	FailInfo     asn1.BitString `asn1:"optional"`
}

type timeStampResponse struct {
	Status         pkiStatusInfo
	TimeStampToken asn1.RawValue `asn1:"optional"`
}

// TstInfo is what a timestamp authority asserts: that the imprint existed at GenTime.
type TstInfo struct {
	Policy        asn1.ObjectIdentifier
	HashAlgorithm asn1.ObjectIdentifier
	HashedMessage []byte
	SerialNumber  *big.Int
	GenTime       time.Time
	Nonce         *big.Int
}

// Token is a verified timestamp token.
type Token struct {
	Der         []byte
	Info        *TstInfo
	Certificate *x509.Certificate
	SignedData  *cms.ParsedSignedData
}

// ParseTstInfo reads a TSTInfo; its optional fields are told apart by tag, which encoding/asn1 cannot do for untagged ones.
func ParseTstInfo(der []byte) (*TstInfo, error) {
	var outer asn1.RawValue
	if _, err := asn1.Unmarshal(der, &outer); err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: asn1 unmarshal (tst info): %w", altshiftErrors.ErrParseError, err))
	}

	var elements []asn1.RawValue
	for rest := outer.Bytes; len(rest) != 0; {
		var element asn1.RawValue
		var err error
		rest, err = asn1.Unmarshal(rest, &element)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: asn1 unmarshal (tst info element): %w", altshiftErrors.ErrParseError, err))
		}
		elements = append(elements, element)
	}
	if len(elements) < 5 {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: tst info has %d elements", altshiftErrors.ErrParseError, len(elements)))
	}

	info := &TstInfo{}
	var imprint messageImprint
	targets := []struct {
		name  string
		value any
	}{
		{"policy", &info.Policy},
		{"message imprint", &imprint},
		{"serial number", &info.SerialNumber},
		{"gen time", &info.GenTime},
	}
	for i, target := range targets {
		if _, err := asn1.Unmarshal(elements[i+1].FullBytes, target.value); err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: asn1 unmarshal (%s): %w", altshiftErrors.ErrParseError, target.name, err))
		}
	}
	info.HashAlgorithm = imprint.HashAlgorithm.Algorithm
	info.HashedMessage = imprint.HashedMessage

	for _, element := range elements[5:] {
		if element.Class == asn1.ClassUniversal && element.Tag == asn1.TagInteger {
			if _, err := asn1.Unmarshal(element.FullBytes, &info.Nonce); err != nil {
				return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: asn1 unmarshal (nonce): %w", altshiftErrors.ErrParseError, err))
			}
		}
	}

	return info, nil
}

// ParseToken reads a timestamp token and checks the TSA's signature over it; it does not check the TSA's certificate chain.
func ParseToken(der []byte) (*Token, error) {
	signedData, err := cms.ParseSignedData(der)
	if err != nil {
		return nil, fmt.Errorf("cms parse signed data: %w", err)
	}
	if !signedData.ContentType.Equal(cms.OidTstInfo) {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: token content type %s is not tst info", altshiftErrors.ErrParseError, signedData.ContentType))
	}

	certificate, err := signedData.Verify(nil)
	if err != nil {
		return nil, fmt.Errorf("signed data verify: %w", err)
	}
	if !slices.Contains(certificate.ExtKeyUsage, x509.ExtKeyUsageTimeStamping) {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: the token's signer is not a timestamping certificate", altshiftErrors.ErrVerificationError))
	}

	info, err := ParseTstInfo(signedData.Content)
	if err != nil {
		return nil, fmt.Errorf("parse tst info: %w", err)
	}

	return &Token{Der: der, Info: info, Certificate: certificate, SignedData: signedData}, nil
}

// Client is a Timestamper speaking RFC 3161 over HTTP.
type Client struct {
	Url string
	// Roots are the trust anchors the authority's certificate has to chain to; a client without them refuses to timestamp.
	Roots *x509.CertPool
	// Intermediates complete that chain where the token does not carry the whole of it.
	Intermediates []*x509.Certificate
	FetchOptions  []fetch_config.Option
	// Nonce makes the request nonce; nil means a random one.
	Nonce func() (*big.Int, error)
}

func badResponse(format string, arguments ...any) error {
	return altshiftErrors.NewWithTrace(fmt.Errorf("%w: %s", ErrBadResponse, fmt.Sprintf(format, arguments...)))
}

func randomNonce() (*big.Int, error) {
	nonce, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), nonceBits))
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("rand int: %w", err))
	}
	return nonce, nil
}

// Timestamp requests a token over a SHA-256 digest and checks that it answers this request.
func (c *Client) Timestamp(ctx context.Context, digest []byte) ([]byte, error) {
	if c == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("tsa client"))
	}
	if c.Url == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("tsa url"))
	}
	if len(digest) != crypto.SHA256.Size() {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: digest is %d bytes, not a SHA-256", altshiftErrors.ErrValidationError, len(digest)))
	}
	if c.Roots == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("tsa trust roots"))
	}

	makeNonce := c.Nonce
	if makeNonce == nil {
		makeNonce = randomNonce
	}
	nonce, err := makeNonce()
	if err != nil {
		return nil, fmt.Errorf("make nonce: %w", err)
	}

	request, err := asn1.Marshal(
		timeStampRequest{
			Version:        1,
			MessageImprint: messageImprint{HashAlgorithm: pkix.AlgorithmIdentifier{Algorithm: cms.OidSha256}, HashedMessage: digest},
			Nonce:          nonce,
			CertReq:        true,
		},
	)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("asn1 marshal (timestamp request): %w", err))
	}

	// The cap goes first, so a caller's own options can raise it.
	options := append(
		append([]fetch_config.Option{fetch_config.WithMaxResponseBodySize(maxResponseSize)}, c.FetchOptions...),
		fetch_config.WithMethod(http.MethodPost),
		fetch_config.WithHeaders(map[string]string{"Content-Type": requestContentType, "Accept": responseContentType}),
		fetch_config.WithBody(request),
	)
	_, responseData, err := altshiftHttpUtils.Fetch(ctx, c.Url, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch (tsa): %w", err), c.Url)
	}

	var response timeStampResponse
	if _, err := asn1.Unmarshal(responseData, &response); err != nil {
		return nil, badResponse("asn1 unmarshal (timestamp response) from %s: %v", c.Url, err)
	}
	// 0 is granted, 1 granted with modifications.
	if response.Status.Status > 1 {
		return nil, badResponse("%s refused the request with status %d", c.Url, response.Status.Status)
	}
	if len(response.TimeStampToken.FullBytes) == 0 {
		return nil, badResponse("%s answered without a token", c.Url)
	}

	token, err := ParseToken(response.TimeStampToken.FullBytes)
	if err != nil {
		// Not wrapped with %w: what the token's parse calls bad input is the service's fault here.
		return nil, badResponse("parse token from %s: %v", c.Url, err)
	}
	if token.Info.Nonce == nil || token.Info.Nonce.Cmp(nonce) != 0 {
		return nil, badResponse("the token's nonce is not the request's")
	}
	if !token.Info.HashAlgorithm.Equal(cms.OidSha256) || !bytes.Equal(token.Info.HashedMessage, digest) {
		return nil, badResponse("the token's imprint is not the request's")
	}

	intermediates := x509.NewCertPool()
	for _, certificate := range slices.Concat(c.Intermediates, token.SignedData.Certificates) {
		if certificate != nil {
			intermediates.AddCert(certificate)
		}
	}
	verifyOptions := x509.VerifyOptions{
		Roots:         c.Roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
		CurrentTime:   token.Info.GenTime,
	}
	if _, err := token.Certificate.Verify(verifyOptions); err != nil {
		return nil, badResponse("the token's signer does not chain to a trusted authority: %v", err)
	}

	return token.Der, nil
}
