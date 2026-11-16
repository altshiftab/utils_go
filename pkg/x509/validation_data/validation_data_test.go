package validation_data

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
)

type node struct {
	certificate *x509.Certificate
	key         crypto.Signer
}

func newNode(t *testing.T, name string, parent *node, isCa bool, issuerUrl string, crlUrl string) *node {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("rand int: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  isCa,
	}
	if isCa {
		template.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	}
	if issuerUrl != "" {
		template.IssuingCertificateURL = []string{issuerUrl}
	}
	if crlUrl != "" {
		template.CRLDistributionPoints = []string{crlUrl}
	}
	issuer, issuerKey := template, crypto.Signer(key)
	if parent != nil {
		issuer, issuerKey = parent.certificate, parent.key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, issuer, key.Public(), issuerKey)
	if err != nil {
		t.Fatalf("x509 create certificate: %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509 parse certificate: %v", err)
	}
	return &node{certificate: certificate, key: key}
}

func (n *node) crl(t *testing.T, thisUpdate time.Time, revoked ...*x509.Certificate) []byte {
	t.Helper()
	list := &x509.RevocationList{Number: big.NewInt(1), ThisUpdate: thisUpdate, NextUpdate: thisUpdate.Add(24 * time.Hour)}
	for _, certificate := range revoked {
		list.RevokedCertificateEntries = append(list.RevokedCertificateEntries, x509.RevocationListEntry{SerialNumber: certificate.SerialNumber, RevocationTime: thisUpdate})
	}
	der, err := x509.CreateRevocationList(rand.Reader, list, n.certificate, n.key)
	if err != nil {
		t.Fatalf("x509 create revocation list: %v", err)
	}
	return der
}

func TestCollectGiven(t *testing.T) {
	t.Parallel()

	now := time.Now()
	root := newNode(t, "root", nil, true, "", "")
	// No URLs: what is given is all there is, so nothing reaches the network.
	issuing := newNode(t, "issuing", root, true, "", "")
	leaf := newNode(t, "leaf", issuing, false, "", "")

	rootCrl := root.crl(t, now)
	issuingCrl := issuing.crl(t, now)

	testCases := []struct {
		name      string
		collector *Collector
		wantCerts int
		wantCrls  int
		wantErr   error
	}{
		{
			name:      "every crl given",
			collector: &Collector{Certificates: []*x509.Certificate{root.certificate, issuing.certificate}, Crls: [][]byte{rootCrl, issuingCrl}},
			wantCerts: 3,
			wantCrls:  2,
		},
		{
			name: "crls issued at collection",
			collector: &Collector{
				Certificates: []*x509.Certificate{root.certificate, issuing.certificate},
				CrlSource:    func(at time.Time) ([][]byte, error) { return [][]byte{root.crl(t, at), issuing.crl(t, at)}, nil },
			},
			wantCerts: 3,
			wantCrls:  2,
		},
		{
			name:      "the leaf is revoked",
			collector: &Collector{Certificates: []*x509.Certificate{root.certificate, issuing.certificate}, Crls: [][]byte{rootCrl, issuing.crl(t, now, leaf.certificate)}},
			wantErr:   ErrRevoked,
		},
		{
			name:      "the crl has lapsed",
			collector: &Collector{Certificates: []*x509.Certificate{root.certificate, issuing.certificate}, Crls: [][]byte{rootCrl, issuing.crl(t, now.Add(-48*time.Hour))}},
			wantErr:   ErrNoCurrentCrl,
		},
		{
			name:      "a crl from another issuer does not count",
			collector: &Collector{Certificates: []*x509.Certificate{root.certificate, issuing.certificate}, Crls: [][]byte{rootCrl, rootCrl}},
			wantErr:   ErrNoCurrentCrl,
		},
		{
			name:      "no issuer to be found",
			collector: &Collector{Crls: [][]byte{rootCrl, issuingCrl}},
			wantErr:   ErrNoIssuer,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			data, err := testCase.collector.Collect(context.Background(), leaf.certificate)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("%s: collect = %v, want %v", testCase.name, err, testCase.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: collect: %v", testCase.name, err)
			}
			if len(data.Certificates) != testCase.wantCerts || len(data.Crls) != testCase.wantCrls {
				t.Errorf("%s: %d certificates and %d crls, want %d and %d", testCase.name, len(data.Certificates), len(data.Crls), testCase.wantCerts, testCase.wantCrls)
			}
		})
	}
}

// TestCollectFetched walks a chain whose issuer and CRL are only reachable at the URLs the certificate names.
func TestCollectFetched(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	root := newNode(t, "root", nil, true, "", "")
	leaf := newNode(t, "leaf", root, false, server.URL+"/root.cer", server.URL+"/root.crl")
	rootCrl := root.crl(t, time.Now())

	var fetched []string
	mux.HandleFunc("/root.cer", func(writer http.ResponseWriter, request *http.Request) {
		fetched = append(fetched, request.URL.Path)
		_, _ = writer.Write(root.certificate.Raw)
	})
	mux.HandleFunc("/root.crl", func(writer http.ResponseWriter, request *http.Request) {
		fetched = append(fetched, request.URL.Path)
		_, _ = writer.Write(rootCrl)
	})

	data, err := (&Collector{}).Collect(context.Background(), leaf.certificate)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(data.Certificates) != 2 || len(data.Crls) != 1 {
		t.Errorf("%d certificates and %d crls, want 2 and 1", len(data.Certificates), len(data.Crls))
	}
	if len(fetched) != 2 {
		t.Errorf("fetched %v, want the issuer and the crl", fetched)
	}
}

// TestCollectFetchFailure reports a CRL that cannot be fetched as missing, with the fetch's failure joined to it.
func TestCollectFetchFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	root := newNode(t, "root", nil, true, "", "")
	leaf := newNode(t, "leaf", root, false, "", server.URL+"/root.crl")

	_, err := (&Collector{Certificates: []*x509.Certificate{root.certificate}}).Collect(context.Background(), leaf.certificate)
	if !errors.Is(err, ErrNoCurrentCrl) {
		t.Errorf("collect = %v, want no current crl", err)
	}
	if errors.Is(err, altshiftErrors.ErrVerificationError) || errors.Is(err, altshiftErrors.ErrParseError) {
		t.Errorf("collect = %v, classed as client input", err)
	}
}

// TestCollectCapsTheCrl does not read a CRL past the cap, and reports the CRL as missing with the reason joined.
func TestCollectCapsTheCrl(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Length", strconv.Itoa(maxCrlResponseSize+1))
	}))
	defer server.Close()

	root := newNode(t, "root", nil, true, "", "")
	leaf := newNode(t, "leaf", root, false, "", server.URL+"/root.crl")

	_, err := (&Collector{Certificates: []*x509.Certificate{root.certificate}}).Collect(context.Background(), leaf.certificate)
	if !errors.Is(err, ErrNoCurrentCrl) || !errors.Is(err, altshiftHttpErrors.ErrResponseBodyTooLarge) {
		t.Errorf("collect = %v, want no current crl because the response was too large", err)
	}
}

func TestCollectDeduplicates(t *testing.T) {
	t.Parallel()

	now := time.Now()
	root := newNode(t, "root", nil, true, "", "")
	first := newNode(t, "first", root, false, "", "")
	second := newNode(t, "second", root, false, "", "")

	collector := &Collector{Certificates: []*x509.Certificate{root.certificate}, Crls: [][]byte{root.crl(t, now)}}
	data, err := collector.Collect(context.Background(), first.certificate, second.certificate)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(data.Certificates) != 3 || len(data.Crls) != 1 {
		t.Errorf("%d certificates and %d crls, want the shared root and its crl once", len(data.Certificates), len(data.Crls))
	}
}
