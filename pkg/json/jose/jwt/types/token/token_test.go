package token

import (
	"errors"
	"testing"

	"github.com/altshiftab/utils_go/pkg/crypto/hmac"
	altshiftCryptoInterfaces "github.com/altshiftab/utils_go/pkg/crypto/interfaces"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

func newSigner(t *testing.T) *hmac.Method {
	t.Helper()
	signer, err := hmac.New("HS256", []byte("secret-key"))
	if err != nil {
		t.Fatalf("hmac new: %v", err)
	}
	return signer
}

func TestEncodeNilSigner(t *testing.T) {
	t.Parallel()

	tok := &Token{Payload: map[string]any{"sub": "abc"}}
	// A one-element slice holds a nil NamedSigner; reading it back yields a nil
	// interface value to exercise Encode's nil-signer guard.
	signers := make([]altshiftCryptoInterfaces.NamedSigner, 1)
	_, err := tok.Encode(signers[0])
	if err == nil {
		t.Fatal("expected error for nil signer")
	}
	if _, ok := errors.AsType[*nil_error.Error](err); !ok {
		t.Errorf("error = %v, want *nil_error.Error", err)
	}
}

func TestEncodeAndNewRoundTripDefaultHeader(t *testing.T) {
	t.Parallel()

	signer := newSigner(t)
	original := &Token{
		Payload: map[string]any{"sub": "abc", "count": float64(5)},
	}

	serialized, err := original.Encode(signer)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := New(serialized)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if decoded == nil {
		t.Fatal("decoded token is nil")
	}

	if decoded.Header["typ"] != "JWT" {
		t.Errorf("header typ = %v, want JWT", decoded.Header["typ"])
	}
	if decoded.Header["alg"] != "HS256" {
		t.Errorf("header alg = %v, want HS256", decoded.Header["alg"])
	}
	if decoded.Payload["sub"] != "abc" {
		t.Errorf("payload sub = %v, want abc", decoded.Payload["sub"])
	}
	if decoded.Payload["count"] != float64(5) {
		t.Errorf("payload count = %v, want 5", decoded.Payload["count"])
	}
}

func TestEncodeWithCustomHeader(t *testing.T) {
	t.Parallel()

	signer := newSigner(t)
	original := &Token{
		Header:  map[string]any{"kid": "key-1"},
		Payload: map[string]any{"a": "b"},
	}

	serialized, err := original.Encode(signer)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := New(serialized)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if decoded == nil {
		t.Fatal("decoded token is nil")
	}

	if decoded.Header["kid"] != "key-1" {
		t.Errorf("header kid = %v, want key-1", decoded.Header["kid"])
	}
	if decoded.Header["alg"] != "HS256" {
		t.Errorf("header alg = %v, want HS256", decoded.Header["alg"])
	}
	if _, ok := decoded.Header["typ"]; ok {
		t.Errorf("header typ present = %v, want absent", decoded.Header["typ"])
	}
}

func TestEncodeDoesNotMutateOriginalHeader(t *testing.T) {
	t.Parallel()

	signer := newSigner(t)
	original := &Token{
		Header:  map[string]any{"kid": "key-1"},
		Payload: map[string]any{"a": "b"},
	}

	if _, err := original.Encode(signer); err != nil {
		t.Fatalf("encode: %v", err)
	}

	if _, ok := original.Header["alg"]; ok {
		t.Errorf("original header was mutated with alg = %v", original.Header["alg"])
	}
}

func TestNewInvalidSerialization(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
	}{
		{name: "no dots", input: "not-a-jwt"},
		{name: "too few parts", input: "aaa.bbb"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tok, err := New(testCase.input)
			if err == nil {
				t.Fatal("expected error")
			}
			if tok != nil {
				t.Errorf("token = %v, want nil", tok)
			}
			if !errors.Is(err, altshiftErrors.ErrParseError) {
				t.Errorf("error = %v, want ErrParseError", err)
			}
		})
	}
}

func TestNewFromJwsNil(t *testing.T) {
	t.Parallel()

	tok, err := NewFromJws(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != nil {
		t.Errorf("token = %v, want nil", tok)
	}
}
