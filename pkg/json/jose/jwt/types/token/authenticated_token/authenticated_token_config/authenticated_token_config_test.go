package authenticated_token_config

import (
	"testing"

	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/token"
)

type stubVerifier struct{}

func (stubVerifier) Verify(_ []byte, _ []byte) error { return nil }

func (stubVerifier) GetName() string { return "stub" }

type stubTokenValidator struct{}

func (stubTokenValidator) Validate(_ *token.Token) error { return nil }

func TestNewDefaults(t *testing.T) {
	t.Parallel()

	config := New()
	if config == nil {
		t.Fatal("New() returned nil")
	}
	if config.SignatureVerifier != nil {
		t.Errorf("default SignatureVerifier = %v, want nil", config.SignatureVerifier)
	}
	if config.AllowUnauthenticated {
		t.Error("default AllowUnauthenticated = true, want false")
	}
	if config.TokenValidator != DefaultValidator {
		t.Errorf("default TokenValidator = %v, want DefaultValidator", config.TokenValidator)
	}
}

func TestWithSignatureVerifier(t *testing.T) {
	t.Parallel()

	verifier := &stubVerifier{}
	config := New(WithSignatureVerifier(verifier))
	if config.SignatureVerifier != verifier {
		t.Errorf("SignatureVerifier = %v, want %v", config.SignatureVerifier, verifier)
	}
}

func TestWithTokenValidator(t *testing.T) {
	t.Parallel()

	tokenValidator := &stubTokenValidator{}
	config := New(WithTokenValidator(tokenValidator))
	if config.TokenValidator != tokenValidator {
		t.Errorf("TokenValidator = %v, want %v", config.TokenValidator, tokenValidator)
	}
}

func TestWithAllowUnauthenticated(t *testing.T) {
	t.Parallel()

	config := New(WithAllowUnauthenticated(true))
	if !config.AllowUnauthenticated {
		t.Error("AllowUnauthenticated = false, want true")
	}
}

func TestDefaultValidatorHasPayloadValidator(t *testing.T) {
	t.Parallel()

	if DefaultValidator == nil {
		t.Fatal("DefaultValidator is nil")
	}
	if DefaultValidator.PayloadValidator == nil {
		t.Error("DefaultValidator.PayloadValidator is nil")
	}
}
