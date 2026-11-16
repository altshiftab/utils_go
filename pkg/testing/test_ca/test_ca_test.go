package test_ca_test

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/testing/test_ca"
	"github.com/altshiftab/utils_go/pkg/tsa"
)

func TestNew(t *testing.T) {
	t.Parallel()

	now := time.Now()
	authority, err := test_ca.New(now, "Test AB")
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	roots := x509.NewCertPool()
	roots.AddCert(authority.Root)
	intermediates := x509.NewCertPool()
	intermediates.AddCert(authority.Issuing)

	testCases := []struct {
		name        string
		certificate *x509.Certificate
		usage       x509.ExtKeyUsage
	}{
		{name: "seal", certificate: authority.Seal, usage: x509.ExtKeyUsageAny},
		{name: "timestamping unit", certificate: authority.Tsa, usage: x509.ExtKeyUsageTimeStamping},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := testCase.certificate.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, KeyUsages: []x509.ExtKeyUsage{testCase.usage}}); err != nil {
				t.Errorf("%s: verify: %v", testCase.name, err)
			}
		})
	}

	if !authority.Seal.PublicKey.(interface{ Equal(crypto.PublicKey) bool }).Equal(authority.SealKey.Public()) {
		t.Error("the sealing key is not the sealing certificate's")
	}
	if authority.Seal.KeyUsage&x509.KeyUsageContentCommitment == 0 {
		t.Error("the sealing certificate lacks non-repudiation")
	}
	if !strings.Contains(authority.Seal.Subject.CommonName, "Test AB") {
		t.Errorf("seal subject %q", authority.Seal.Subject.CommonName)
	}

	block, _ := pem.Decode(authority.RootPem())
	if block == nil || !bytes.Equal(block.Bytes, authority.Root.Raw) {
		t.Error("root pem is not the root")
	}

	if _, err := test_ca.New(now, ""); err == nil {
		t.Error("new without an organization succeeded")
	}
}

func TestCrls(t *testing.T) {
	t.Parallel()

	now := time.Now().Truncate(time.Second)
	authority, err := test_ca.New(now, "Test AB")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	crls, err := authority.Crls(now)
	if err != nil {
		t.Fatalf("crls: %v", err)
	}

	issuers := []*x509.Certificate{authority.Root, authority.Issuing}
	if len(crls) != len(issuers) {
		t.Fatalf("%d crls, want one per CA", len(crls))
	}
	for i, der := range crls {
		crl, err := x509.ParseRevocationList(der)
		if err != nil {
			t.Fatalf("parse revocation list: %v", err)
		}
		if err := crl.CheckSignatureFrom(issuers[i]); err != nil {
			t.Errorf("crl %d: check signature: %v", i, err)
		}
		if !crl.ThisUpdate.Equal(now) || len(crl.RevokedCertificateEntries) != 0 {
			t.Errorf("crl %d: this update %v with %d entries", i, crl.ThisUpdate, len(crl.RevokedCertificateEntries))
		}
	}
}

func TestIssueToken(t *testing.T) {
	t.Parallel()

	authority, err := test_ca.New(time.Now(), "Test AB")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	digest := sha256.Sum256([]byte("x"))
	at := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

	der, err := authority.IssueToken(digest[:], big.NewInt(7), at)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	token, err := tsa.ParseToken(der)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if !token.Certificate.Equal(authority.Tsa) || !token.Info.GenTime.Equal(at) || token.Info.Nonce.Int64() != 7 || !bytes.Equal(token.Info.HashedMessage, digest[:]) {
		t.Errorf("token %+v", token.Info)
	}
}

// TestServeHttpRefusesGarbage answers a request it cannot read with a rejection rather than a token.
func TestServeHttpRefusesGarbage(t *testing.T) {
	t.Parallel()

	authority, err := test_ca.New(time.Now(), "Test AB")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	recorder := httptest.NewRecorder()
	authority.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("not asn.1")))

	var response struct {
		Status struct{ Status int }
	}
	if _, err := asn1.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("asn1 unmarshal: %v", err)
	}
	if response.Status.Status != 2 {
		t.Errorf("status %d, want rejection", response.Status.Status)
	}
}

// TestIssueSeal issues a sealing certificate for a key held elsewhere, chained like the authority's own.
func TestIssueSeal(t *testing.T) {
	t.Parallel()

	authority, err := test_ca.New(time.Now(), "Test AB")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	elsewhere, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}

	certificate, err := authority.IssueSeal(elsewhere.Public())
	if err != nil {
		t.Fatalf("issue seal: %v", err)
	}
	if !certificate.PublicKey.(interface{ Equal(crypto.PublicKey) bool }).Equal(elsewhere.Public()) {
		t.Error("the certificate is not for the key given")
	}
	roots := x509.NewCertPool()
	roots.AddCert(authority.Root)
	intermediates := x509.NewCertPool()
	intermediates.AddCert(authority.Issuing)
	if _, err := certificate.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates}); err != nil {
		t.Errorf("verify: %v", err)
	}
	if certificate.Subject.CommonName != authority.Seal.Subject.CommonName {
		t.Errorf("subject %q, want the authority's seal subject", certificate.Subject.CommonName)
	}
	if _, err := authority.IssueSeal(nil); err == nil {
		t.Error("issue seal without a key succeeded")
	}
}
