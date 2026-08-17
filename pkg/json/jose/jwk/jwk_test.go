package jwk

import (
	"errors"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/mismatch_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		keyMap       map[string]any
		wantErr      bool
		wantIs       []error
		wantNilError bool
		wantMismatch bool
	}{
		{
			name:         "nil map",
			keyMap:       nil,
			wantErr:      true,
			wantIs:       []error{altshiftErrors.ErrValidationError},
			wantNilError: true,
		},
		{
			name:   "rsa rs256 valid",
			keyMap: map[string]any{"kty": "RSA", "alg": "RS256"},
		},
		{
			name:   "rsa ps256 valid",
			keyMap: map[string]any{"kty": "RSA", "alg": "PS256"},
		},
		{
			name:   "ec es256 with crv valid",
			keyMap: map[string]any{"kty": "EC", "alg": "ES256", "crv": "P-256"},
		},
		{
			name:    "ec es256 missing crv",
			keyMap:  map[string]any{"kty": "EC", "alg": "ES256"},
			wantErr: true,
			wantIs:  []error{altshiftErrors.ErrValidationError},
		},
		{
			name:         "kty ec but rsa alg mismatch",
			keyMap:       map[string]any{"kty": "EC", "alg": "RS256"},
			wantErr:      true,
			wantIs:       []error{altshiftErrors.ErrVerificationError},
			wantMismatch: true,
		},
		{
			name:         "kty rsa but ec alg mismatch",
			keyMap:       map[string]any{"kty": "RSA", "alg": "ES256"},
			wantErr:      true,
			wantIs:       []error{altshiftErrors.ErrVerificationError},
			wantMismatch: true,
		},
		{
			name:   "unknown alg prefix skips kty check",
			keyMap: map[string]any{"kty": "oct", "alg": "HS256"},
		},
		{
			name:    "missing kty",
			keyMap:  map[string]any{"alg": "RS256"},
			wantErr: true,
			wantIs:  []error{altshiftErrors.ErrValidationError},
		},
		{
			name:    "missing alg",
			keyMap:  map[string]any{"kty": "RSA"},
			wantErr: true,
			wantIs:  []error{altshiftErrors.ErrValidationError},
		},
		{
			name:    "kty wrong type",
			keyMap:  map[string]any{"kty": 123, "alg": "RS256"},
			wantErr: true,
			wantIs:  []error{altshiftErrors.ErrValidationError},
		},
		{
			name:    "alg wrong type",
			keyMap:  map[string]any{"kty": "RSA", "alg": 123},
			wantErr: true,
			wantIs:  []error{altshiftErrors.ErrValidationError},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := Validate(testCase.keyMap)

			if !testCase.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatal("expected error but got nil")
			}
			for _, target := range testCase.wantIs {
				if !errors.Is(err, target) {
					t.Fatalf("errors.Is(%v) = false, want true", target)
				}
			}
			if testCase.wantNilError {
				if _, ok := errors.AsType[*nil_error.Error](err); !ok {
					t.Fatalf("expected *nil_error.Error in chain, got %v", err)
				}
			}
			if testCase.wantMismatch {
				if _, ok := errors.AsType[*mismatch_error.Error](err); !ok {
					t.Fatalf("expected *mismatch_error.Error in chain, got %v", err)
				}
			}
		})
	}
}
