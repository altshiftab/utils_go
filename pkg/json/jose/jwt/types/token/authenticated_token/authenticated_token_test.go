package authenticated_token

import (
	"errors"
	"testing"

	"github.com/altshiftab/utils_go/pkg/crypto/hmac"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftJwtToken "github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/token"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/token/authenticated_token/authenticated_token_config"
)

func newHmac(t *testing.T, algorithm string, secret string) *hmac.Method {
	t.Helper()
	method, err := hmac.New(algorithm, []byte(secret))
	if err != nil {
		t.Fatalf("hmac new (%s): %v", algorithm, err)
	}
	return method
}

func signToken(t *testing.T, signer *hmac.Method, payload map[string]any) string {
	t.Helper()
	tok := &altshiftJwtToken.Token{Payload: payload}
	serialized, err := tok.Encode(signer)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return serialized
}

func TestNewEmptyString(t *testing.T) {
	t.Parallel()

	tok, err := New("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != nil {
		t.Errorf("token = %v, want nil", tok)
	}
}

func TestNewInvalidSerialization(t *testing.T) {
	t.Parallel()

	tok, err := New("not-a-jwt")
	if err == nil {
		t.Fatal("expected error")
	}
	if tok != nil {
		t.Errorf("token = %v, want nil", tok)
	}
	if !errors.Is(err, altshiftErrors.ErrParseError) {
		t.Errorf("error = %v, want ErrParseError", err)
	}
}

func TestNewAllowUnauthenticated(t *testing.T) {
	t.Parallel()

	signer := newHmac(t, "HS256", "secret")
	serialized := signToken(t, signer, map[string]any{"sub": "user"})

	tok, err := New(serialized, authenticated_token_config.WithAllowUnauthenticated(true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok == nil {
		t.Fatal("expected non-nil token")
	}
	if tok.Raw() != serialized {
		t.Errorf("Raw() = %q, want %q", tok.Raw(), serialized)
	}
	if tok.Verifier() != nil {
		t.Errorf("Verifier() = %v, want nil", tok.Verifier())
	}
	if tok.Payload["sub"] != "user" {
		t.Errorf("payload sub = %v, want user", tok.Payload["sub"])
	}
}

func TestNewMissingVerifierNotAllowed(t *testing.T) {
	t.Parallel()

	signer := newHmac(t, "HS256", "secret")
	serialized := signToken(t, signer, map[string]any{"sub": "user"})

	tok, err := New(serialized)
	if err == nil {
		t.Fatal("expected error for missing verifier")
	}
	if tok == nil {
		t.Error("expected token to still be returned alongside error")
	}
	if _, ok := errors.AsType[*nil_error.Error](err); !ok {
		t.Errorf("error = %v, want *nil_error.Error", err)
	}
}

func TestNewWithMatchingVerifier(t *testing.T) {
	t.Parallel()

	signer := newHmac(t, "HS256", "secret")
	verifier := newHmac(t, "HS256", "secret")
	serialized := signToken(t, signer, map[string]any{"sub": "user"})

	tok, err := New(serialized, authenticated_token_config.WithSignatureVerifier(verifier))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok == nil {
		t.Fatal("expected non-nil token")
	}
	if tok.Verifier() != verifier {
		t.Errorf("Verifier() = %v, want %v", tok.Verifier(), verifier)
	}
}

func TestNewAlgMismatch(t *testing.T) {
	t.Parallel()

	signer := newHmac(t, "HS256", "secret")
	// Verifier reports HS384 while the token header carries alg=HS256.
	verifier := newHmac(t, "HS384", "secret")
	serialized := signToken(t, signer, map[string]any{"sub": "user"})

	_, err := New(serialized, authenticated_token_config.WithSignatureVerifier(verifier))
	if err == nil {
		t.Fatal("expected error for alg mismatch")
	}
	if !errors.Is(err, altshiftErrors.ErrVerificationError) {
		t.Errorf("error = %v, want ErrVerificationError", err)
	}
}

func TestNewBadSignature(t *testing.T) {
	t.Parallel()

	signer := newHmac(t, "HS256", "secret-a")
	verifier := newHmac(t, "HS256", "secret-b")
	serialized := signToken(t, signer, map[string]any{"sub": "user"})

	_, err := New(serialized, authenticated_token_config.WithSignatureVerifier(verifier))
	if err == nil {
		t.Fatal("expected error for bad signature")
	}
	if !errors.Is(err, altshiftErrors.ErrVerificationError) {
		t.Errorf("error = %v, want ErrVerificationError", err)
	}
}
