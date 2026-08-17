package utils

import (
	"crypto/x509"
	"net"
	"net/url"
	"testing"
)

func TestExtractAlternativeNamesNil(t *testing.T) {
	t.Parallel()

	if got := ExtractAlternativeNames(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestExtractAlternativeNamesEmpty(t *testing.T) {
	t.Parallel()

	if got := ExtractAlternativeNames(&x509.Certificate{}); len(got) != 0 {
		t.Fatalf("expected no names, got %v", got)
	}
}

func TestExtractAlternativeNames(t *testing.T) {
	t.Parallel()

	certificate := &x509.Certificate{
		DNSNames:       []string{"example.com", "example.com", "www.example.com"},
		IPAddresses:    []net.IP{net.ParseIP("192.0.2.1")},
		EmailAddresses: []string{"admin@example.com"},
		URIs:           []*url.URL{{Scheme: "https", Host: "example.com", Path: "/"}},
	}

	got := ExtractAlternativeNames(certificate)

	expected := map[string]struct{}{
		"example.com":          {},
		"www.example.com":      {},
		"192.0.2.1":            {},
		"admin@example.com":    {},
		"https://example.com/": {},
	}

	if len(got) != len(expected) {
		t.Fatalf("expected %d unique names, got %d: %v", len(expected), len(got), got)
	}

	for _, name := range got {
		if _, ok := expected[name]; !ok {
			t.Fatalf("unexpected name %q in result %v", name, got)
		}
	}
}
