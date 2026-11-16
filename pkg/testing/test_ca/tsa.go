package test_ca

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"

	"github.com/altshiftab/utils_go/pkg/cms"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

// tsaPolicy is an arbitrary policy OID under the private-use arc reserved for examples.
var tsaPolicy = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 32473, 1}

type messageImprint struct {
	HashAlgorithm pkix.AlgorithmIdentifier
	HashedMessage []byte
}

type tstInfo struct {
	Version        int
	Policy         asn1.ObjectIdentifier
	MessageImprint messageImprint
	SerialNumber   *big.Int
	GenTime        time.Time `asn1:"generalized"`
	Nonce          *big.Int  `asn1:"optional"`
}

type timeStampRequest struct {
	Version        int
	MessageImprint messageImprint
	Nonce          *big.Int `asn1:"optional"`
	CertReq        bool     `asn1:"optional"`
}

type pkiStatusInfo struct {
	Status int
}

type timeStampResponse struct {
	Status         pkiStatusInfo
	TimeStampToken asn1.RawValue `asn1:"optional"`
}

// IssueToken makes a timestamp token over a SHA-256 digest at the given time.
func (a *Authority) IssueToken(digest []byte, nonce *big.Int, at time.Time) ([]byte, error) {
	if a == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("authority"))
	}
	serial, err := serialNumber()
	if err != nil {
		return nil, fmt.Errorf("serial number: %w", err)
	}
	content, err := asn1.Marshal(
		tstInfo{
			Version:        1,
			Policy:         tsaPolicy,
			MessageImprint: messageImprint{HashAlgorithm: pkix.AlgorithmIdentifier{Algorithm: cms.OidSha256}, HashedMessage: digest},
			SerialNumber:   serial,
			GenTime:        at.UTC().Truncate(time.Second),
			Nonce:          nonce,
		},
	)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("asn1 marshal (tst info): %w", err))
	}
	contentDigest := sha256.Sum256(content)
	signerInfo, err := cms.NewSignerInfo(a.tsaKey, a.Tsa, cms.OidTstInfo, contentDigest[:])
	if err != nil {
		return nil, fmt.Errorf("cms new signer info: %w", err)
	}
	token, err := cms.MarshalSignedData(cms.OidTstInfo, content, []*x509.Certificate{a.Tsa}, signerInfo)
	if err != nil {
		return nil, fmt.Errorf("cms marshal signed data: %w", err)
	}
	return token, nil
}

// Timestamp makes the authority a tsa.Timestamper that answers in process.
func (a *Authority) Timestamp(_ context.Context, digest []byte) ([]byte, error) {
	return a.IssueToken(digest, nil, time.Now())
}

func (a *Authority) respond(request []byte) ([]byte, error) {
	var parsed timeStampRequest
	if _, err := asn1.Unmarshal(request, &parsed); err != nil {
		return asn1.Marshal(timeStampResponse{Status: pkiStatusInfo{Status: 2}})
	}
	token, err := a.IssueToken(parsed.MessageImprint.HashedMessage, parsed.Nonce, time.Now())
	if err != nil {
		return nil, fmt.Errorf("issue token: %w", err)
	}
	return asn1.Marshal(timeStampResponse{Status: pkiStatusInfo{Status: 0}, TimeStampToken: asn1.RawValue{FullBytes: token}})
}

// ServeHTTP answers RFC 3161 requests, so the authority can stand in for a TSA over HTTP.
func (a *Authority) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(io.LimitReader(request.Body, 1<<16))
	if err != nil {
		http.Error(writer, "read body", http.StatusBadRequest)
		return
	}
	response, err := a.respond(body)
	if err != nil {
		http.Error(writer, "respond", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/timestamp-reply")
	_, _ = writer.Write(response)
}
