package hmac

import (
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"testing"

	motmedelCryptoErrors "github.com/altshiftab/utils_go/pkg/crypto/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
)

func TestNew(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		alg      string
		wantName string
	}{
		{name: "HS256", alg: "HS256", wantName: "HS256"},
		{name: "HS384", alg: "HS384", wantName: "HS384"},
		{name: "HS512", alg: "HS512", wantName: "HS512"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m, err := New(tc.alg, []byte("secret"))
			if err != nil {
				t.Fatalf("New(%q): %v", tc.alg, err)
			}
			if m == nil {
				t.Fatalf("New(%q) returned nil method", tc.alg)
			}
			if m.GetName() != tc.wantName {
				t.Fatalf("GetName(): got %q want %q", m.GetName(), tc.wantName)
			}
			if m.HashFunc == nil {
				t.Fatalf("HashFunc is nil")
			}
			if string(m.Secret) != "secret" {
				t.Fatalf("secret mismatch")
			}
		})
	}
}

func TestNew_UnsupportedAlgorithm(t *testing.T) {
	t.Parallel()

	_, err := New("HS999", []byte("secret"))
	if err == nil {
		t.Fatal("expected error for unsupported algorithm")
	}
	if !errors.Is(err, motmedelCryptoErrors.ErrUnsupportedAlgorithm) {
		t.Fatalf("expected ErrUnsupportedAlgorithm, got %v", err)
	}
}

func TestSignVerify_RoundTrip(t *testing.T) {
	t.Parallel()

	algs := []string{"HS256", "HS384", "HS512"}
	for _, alg := range algs {
		alg := alg
		t.Run(alg, func(t *testing.T) {
			t.Parallel()

			m, err := New(alg, []byte("topsecret"))
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			message := []byte("hello world")
			sig, err := m.Sign(message)
			if err != nil {
				t.Fatalf("Sign: %v", err)
			}
			if len(sig) == 0 {
				t.Fatal("empty signature")
			}

			if err := m.Verify(message, sig); err != nil {
				t.Fatalf("Verify (matching): %v", err)
			}
		})
	}
}

func TestSign_EmptySecret(t *testing.T) {
	t.Parallel()

	m := &Method{HashFunc: sha256.New}
	_, err := m.Sign([]byte("hello"))
	if err == nil {
		t.Fatal("expected error for empty secret")
	}
	ee, ok := errors.AsType[*empty_error.Error](err)
	if !ok {
		t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
	}
	if ee.Field != "secret" {
		t.Errorf("Field = %q, want %q", ee.Field, "secret")
	}
}

func TestVerify_EmptySecret(t *testing.T) {
	t.Parallel()

	m := &Method{HashFunc: sha512.New}
	err := m.Verify([]byte("hello"), []byte{0x00})
	if err == nil {
		t.Fatal("expected error for empty secret")
	}
	ee, ok := errors.AsType[*empty_error.Error](err)
	if !ok {
		t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
	}
	if ee.Field != "secret" {
		t.Errorf("Field = %q, want %q", ee.Field, "secret")
	}
}

func TestVerify_SignatureMismatch(t *testing.T) {
	t.Parallel()

	m, err := New("HS256", []byte("secret"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = m.Verify([]byte("hello"), []byte("not the signature"))
	if !errors.Is(err, motmedelCryptoErrors.ErrSignatureMismatch) {
		t.Fatalf("expected ErrSignatureMismatch, got %v", err)
	}
}

func TestVerify_DifferentSecretFails(t *testing.T) {
	t.Parallel()

	signer, err := New("HS256", []byte("secret-a"))
	if err != nil {
		t.Fatalf("New (signer): %v", err)
	}
	verifier, err := New("HS256", []byte("secret-b"))
	if err != nil {
		t.Fatalf("New (verifier): %v", err)
	}

	sig, err := signer.Sign([]byte("hello"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	err = verifier.Verify([]byte("hello"), sig)
	if !errors.Is(err, motmedelCryptoErrors.ErrSignatureMismatch) {
		t.Fatalf("expected ErrSignatureMismatch, got %v", err)
	}
}

func TestGetName(t *testing.T) {
	t.Parallel()

	m := &Method{Name: "custom"}
	if got := m.GetName(); got != "custom" {
		t.Fatalf("GetName: got %q want %q", got, "custom")
	}
}
