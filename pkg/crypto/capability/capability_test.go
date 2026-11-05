package capability

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

const testDomain = "test-service/download/v1"

type testGrant struct {
	Account   string `json:"a"`
	MessageId string `json:"m,omitzero"`
}

func TestSigner(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	key := bytes.Repeat([]byte{7}, MinKeyBytes)
	signer, err := NewSigner[testGrant](key, testDomain, 10*time.Minute, clock)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	otherKey, err := NewSigner[testGrant](bytes.Repeat([]byte{8}, MinKeyBytes), testDomain, 10*time.Minute, clock)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	otherDomain, err := NewSigner[testGrant](key, "other-service/download/v1", 10*time.Minute, clock)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}

	grant := &testGrant{Account: "a@example.com", MessageId: "m1"}
	token, expiresAt, err := signer.Mint(grant)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if !expiresAt.Equal(now.Add(10 * time.Minute)) {
		t.Errorf("expires at %v", expiresAt)
	}
	payload, signature, _ := strings.Cut(token, ".")

	// A payload naming another account, under the original signature.
	data, _ := base64.RawURLEncoding.DecodeString(payload)
	forged := base64.RawURLEncoding.EncodeToString(bytes.Replace(data, []byte("a@example.com"), []byte("b@example.com"), 1)) + "." + signature

	// Tokens that are well signed but carry no grant, or no expiry.
	signed := func(raw string) string {
		encoded := base64.RawURLEncoding.EncodeToString([]byte(raw))
		return encoded + "." + base64.RawURLEncoding.EncodeToString(signer.mac(encoded))
	}

	otherKeyToken, _, err := otherKey.Mint(grant)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	otherDomainToken, _, err := otherDomain.Mint(grant)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	testCases := []struct {
		name    string
		token   string
		at      time.Time
		wantErr error
	}{
		{name: "fresh", token: token, at: now},
		{name: "just before it runs out", token: token, at: now.Add(10*time.Minute - time.Second)},
		{name: "run out", token: token, at: now.Add(10 * time.Minute), wantErr: ErrExpired},
		{name: "another key's", token: otherKeyToken, at: now, wantErr: ErrBadSignature},
		{name: "the same key's in another domain", token: otherDomainToken, at: now, wantErr: ErrBadSignature},
		{name: "a payload changed", token: forged, at: now, wantErr: ErrBadSignature},
		{name: "no signature", token: payload, at: now, wantErr: ErrMalformed},
		{name: "a signature that is not base64", token: payload + ".!!", at: now, wantErr: ErrMalformed},
		{name: "empty", token: "", at: now, wantErr: ErrMalformed},
		{name: "signed, without a grant", token: signed(`{"e":1893456000}`), at: now, wantErr: ErrMalformed},
		{name: "signed, without an expiry", token: signed(`{"g":{"a":"a@example.com"}}`), at: now, wantErr: ErrMalformed},
		{name: "signed, not json", token: signed(`not json`), at: now, wantErr: ErrMalformed},
		{name: "signed, with an unknown member", token: signed(`{"g":{"a":"a@example.com"},"e":1893456000,"x":1}`), at: now, wantErr: ErrMalformed},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			verifier, err := NewSigner[testGrant](key, testDomain, 10*time.Minute, func() time.Time { return testCase.at })
			if err != nil {
				t.Fatalf("new signer: %v", err)
			}
			got, err := verifier.Verify(testCase.token)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) || !errors.Is(err, altshiftErrors.ErrVerificationError) {
					t.Errorf("%s: err = %v, want %v", testCase.name, err, testCase.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", testCase.name, err)
			}
			if got == nil || *got != *grant {
				t.Errorf("%s: grant = %+v", testCase.name, got)
			}
		})
	}
}

func TestNewSigner(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		key     []byte
		domain  string
		wantErr bool
	}{
		{name: "a key and a domain", key: make([]byte, MinKeyBytes), domain: testDomain},
		{name: "a short key", key: make([]byte, MinKeyBytes-1), domain: testDomain, wantErr: true},
		{name: "no domain", key: make([]byte, MinKeyBytes), wantErr: true},
		{name: "a domain holding a NUL", key: make([]byte, MinKeyBytes), domain: "a\x00b", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			signer, err := NewSigner[testGrant](testCase.key, testCase.domain, 0, nil)
			if testCase.wantErr {
				if err == nil {
					t.Errorf("%s: no error", testCase.name)
				}
				return
			}
			if err != nil || signer == nil {
				t.Errorf("%s: %v", testCase.name, err)
			}
		})
	}
}

// A zero TTL is the default one.
func TestNewSigner_DefaultTtl(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	signer, err := NewSigner[testGrant](make([]byte, MinKeyBytes), testDomain, 0, func() time.Time { return now })
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	_, expiresAt, err := signer.Mint(&testGrant{Account: "a"})
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if !expiresAt.Equal(now.Add(DefaultTtl)) {
		t.Errorf("expires at %v, want %v", expiresAt, now.Add(DefaultTtl))
	}
}

func TestMint_RefusesNil(t *testing.T) {
	t.Parallel()

	signer, err := NewSigner[testGrant](make([]byte, MinKeyBytes), testDomain, 0, nil)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	if _, _, err := signer.Mint(nil); err == nil {
		t.Error("expected a nil grant to be refused")
	}
}
