package tsa_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/cms"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
	"github.com/altshiftab/utils_go/pkg/testing/test_ca"
	"github.com/altshiftab/utils_go/pkg/tsa"
)

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
	Status int
}

type timeStampResponse struct {
	Status         pkiStatusInfo
	TimeStampToken asn1.RawValue `asn1:"optional"`
}

type tstInfo struct {
	Version        int
	Policy         asn1.ObjectIdentifier
	MessageImprint messageImprint
	SerialNumber   *big.Int
	GenTime        time.Time `asn1:"generalized"`
	Nonce          *big.Int  `asn1:"optional"`
}

// TestParseTokenFromSk reads a token SK's DEMO service issued, which is signed with ECDSA over SHA-512.
func TestParseTokenFromSk(t *testing.T) {
	t.Parallel()

	der, err := os.ReadFile("testdata/sk_demo_token.der")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	wantImprint, _ := hex.DecodeString("f2ca1bb6c7e907d06dafe4687e579fce76b37e4e93b7605022da52e6ccc26fd2")
	wantNonce, _ := new(big.Int).SetString("C93693A6BC388B9F", 16)

	tampered := bytes.Clone(der)
	tampered[len(tampered)-5] ^= 0x01

	testCases := []struct {
		name    string
		der     []byte
		wantErr bool
	}{
		{name: "as issued", der: der},
		{name: "signature changed", der: tampered, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			token, err := tsa.ParseToken(testCase.der)
			if testCase.wantErr {
				if err == nil {
					t.Errorf("%s: parse token succeeded", testCase.name)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: parse token: %v", testCase.name, err)
			}
			if !bytes.Equal(token.Info.HashedMessage, wantImprint) || !token.Info.HashAlgorithm.Equal(cms.OidSha256) {
				t.Errorf("%s: imprint %x", testCase.name, token.Info.HashedMessage)
			}
			if token.Info.Nonce == nil || token.Info.Nonce.Cmp(wantNonce) != 0 {
				t.Errorf("%s: nonce %v", testCase.name, token.Info.Nonce)
			}
			if want := time.Date(2026, 11, 16, 12, 30, 18, 0, time.UTC); !token.Info.GenTime.Equal(want) {
				t.Errorf("%s: gen time %v, want %v", testCase.name, token.Info.GenTime, want)
			}
			if token.Certificate.Subject.CommonName != "DEMO SK TIMESTAMPING UNIT 2025E" {
				t.Errorf("%s: signer %q", testCase.name, token.Certificate.Subject.CommonName)
			}
		})
	}
}

// answer is a TSA that misbehaves on request: it can stamp another digest, echo another nonce, or refuse.
func answer(t *testing.T, authority *test_ca.Authority, status int, otherDigest bool, otherNonce bool) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		var parsed timeStampRequest
		if _, err := asn1.Unmarshal(body, &parsed); err != nil {
			t.Errorf("asn1 unmarshal request: %v", err)
			return
		}
		if request.Header.Get("Content-Type") != "application/timestamp-query" {
			t.Errorf("content type %q", request.Header.Get("Content-Type"))
		}
		response := timeStampResponse{Status: pkiStatusInfo{Status: status}}
		if status <= 1 {
			digest, nonce := parsed.MessageImprint.HashedMessage, parsed.Nonce
			if otherDigest {
				other := sha256.Sum256([]byte("something else"))
				digest = other[:]
			}
			if otherNonce {
				nonce = new(big.Int).Add(nonce, big.NewInt(1))
			}
			token, err := authority.IssueToken(digest, nonce, time.Now())
			if err != nil {
				t.Errorf("issue token: %v", err)
				return
			}
			response.TimeStampToken = asn1.RawValue{FullBytes: token}
		}
		der, _ := asn1.Marshal(response)
		_, _ = writer.Write(der)
	})
}

func TestClientTimestamp(t *testing.T) {
	t.Parallel()

	authority, err := test_ca.New(time.Now(), "Test AB")
	if err != nil {
		t.Fatalf("test ca new: %v", err)
	}
	stranger, err := test_ca.New(time.Now(), "Other AB")
	if err != nil {
		t.Fatalf("test ca new: %v", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(authority.Root)
	strangerRoots := x509.NewCertPool()
	strangerRoots.AddCert(stranger.Root)
	digest := sha256.Sum256([]byte("signature value"))

	testCases := []struct {
		name          string
		handler       http.Handler
		roots         *x509.CertPool
		intermediates []*x509.Certificate
		digest        []byte
		wantErr       error
	}{
		{name: "granted", handler: authority, roots: roots, intermediates: []*x509.Certificate{authority.Issuing}, digest: digest[:]},
		{name: "another digest stamped", handler: answer(t, authority, 0, true, false), roots: roots, intermediates: []*x509.Certificate{authority.Issuing}, digest: digest[:], wantErr: tsa.ErrBadResponse},
		{name: "another nonce echoed", handler: answer(t, authority, 0, false, true), roots: roots, intermediates: []*x509.Certificate{authority.Issuing}, digest: digest[:], wantErr: tsa.ErrBadResponse},
		{name: "refused", handler: answer(t, authority, 2, false, false), roots: roots, intermediates: []*x509.Certificate{authority.Issuing}, digest: digest[:], wantErr: tsa.ErrBadResponse},
		{name: "an authority that does not chain to the roots", handler: authority, roots: strangerRoots, intermediates: []*x509.Certificate{authority.Issuing}, digest: digest[:], wantErr: tsa.ErrBadResponse},
		{name: "a chain with its intermediate missing", handler: authority, roots: roots, digest: digest[:], wantErr: tsa.ErrBadResponse},
		{name: "no roots", handler: authority, digest: digest[:]},
		{name: "not a sha-256 digest", handler: authority, roots: roots, digest: digest[:20], wantErr: altshiftErrors.ErrValidationError},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(testCase.handler)
			defer server.Close()

			client := &tsa.Client{Url: server.URL, Roots: testCase.roots, Intermediates: testCase.intermediates}
			der, err := client.Timestamp(context.Background(), testCase.digest)
			if testCase.roots == nil {
				if err == nil {
					t.Errorf("%s: timestamp without trust roots succeeded", testCase.name)
				}
				return
			}
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("%s: timestamp = %v, want %v", testCase.name, err, testCase.wantErr)
				}
				// A failure of the service is not the caller's bad input.
				if errors.Is(testCase.wantErr, tsa.ErrBadResponse) && (errors.Is(err, altshiftErrors.ErrParseError) || errors.Is(err, altshiftErrors.ErrVerificationError) || errors.Is(err, altshiftErrors.ErrValidationError)) {
					t.Errorf("%s: %v is classed as client input", testCase.name, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: timestamp: %v", testCase.name, err)
			}
			token, err := tsa.ParseToken(der)
			if err != nil {
				t.Fatalf("%s: parse token: %v", testCase.name, err)
			}
			if !bytes.Equal(token.Info.HashedMessage, testCase.digest) {
				t.Errorf("%s: imprint %x, want %x", testCase.name, token.Info.HashedMessage, testCase.digest)
			}
		})
	}
}

// TestParseTokenRequiresTimestampingCertificate refuses a token signed by a certificate not issued for timestamping.
func TestParseTokenRequiresTimestampingCertificate(t *testing.T) {
	t.Parallel()

	authority, err := test_ca.New(time.Now(), "Test AB")
	if err != nil {
		t.Fatalf("test ca new: %v", err)
	}
	digest := sha256.Sum256(nil)
	content, err := asn1.Marshal(tstInfo{
		Version:        1,
		Policy:         asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 32473, 1},
		MessageImprint: messageImprint{HashAlgorithm: pkix.AlgorithmIdentifier{Algorithm: cms.OidSha256}, HashedMessage: digest[:]},
		SerialNumber:   big.NewInt(1),
		GenTime:        time.Now().UTC().Truncate(time.Second),
	})
	if err != nil {
		t.Fatalf("asn1 marshal: %v", err)
	}
	contentDigest := sha256.Sum256(content)
	signerInfo, err := cms.NewSignerInfo(authority.SealKey, authority.Seal, cms.OidTstInfo, contentDigest[:])
	if err != nil {
		t.Fatalf("new signer info: %v", err)
	}
	token, err := cms.MarshalSignedData(cms.OidTstInfo, content, []*x509.Certificate{authority.Seal}, signerInfo)
	if err != nil {
		t.Fatalf("marshal signed data: %v", err)
	}

	if _, err := tsa.ParseToken(token); !errors.Is(err, altshiftErrors.ErrVerificationError) {
		t.Errorf("parse token = %v, want a verification error", err)
	}
}

// TestClientTimestampCapsTheResponse refuses a service that sends far more than any token, rather than reading it all.
func TestClientTimestampCapsTheResponse(t *testing.T) {
	t.Parallel()

	authority, err := test_ca.New(time.Now(), "Test AB")
	if err != nil {
		t.Fatalf("test ca new: %v", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(authority.Root)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Length", strconv.Itoa(64<<20))
	}))
	defer server.Close()

	digest := sha256.Sum256(nil)
	client := &tsa.Client{Url: server.URL, Roots: roots}
	if _, err := client.Timestamp(context.Background(), digest[:]); !errors.Is(err, altshiftHttpErrors.ErrResponseBodyTooLarge) {
		t.Errorf("timestamp = %v, want too large", err)
	}
}
