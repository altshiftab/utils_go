package rsa

import (
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"math/big"
	"testing"

	motmedelJwkErrors "github.com/altshiftab/utils_go/pkg/json/jose/jwk/errors"
)

func TestKey_PublicKey(t *testing.T) {
	t.Parallel()

	// Valid RSA public key components (small example for testing)
	validN := base64.RawURLEncoding.EncodeToString(big.NewInt(3233).Bytes()) // n = 3233 = 61 * 53
	validE := base64.RawURLEncoding.EncodeToString(big.NewInt(17).Bytes())   // e = 17

	testCases := []struct {
		name        string
		key         *Key
		expectError bool
		validate    func(*rsa.PublicKey) bool
	}{
		{
			name: "valid key",
			key: &Key{
				N: validN,
				E: validE,
			},
			expectError: false,
			validate: func(pk *rsa.PublicKey) bool {
				return pk.N.Cmp(big.NewInt(3233)) == 0 && pk.E == 17
			},
		},
		{
			name: "invalid base64 N",
			key: &Key{
				N: "!!!invalid!!!",
				E: validE,
			},
			expectError: true,
		},
		{
			name: "invalid base64 E",
			key: &Key{
				N: validN,
				E: "!!!invalid!!!",
			},
			expectError: true,
		},
		{
			name: "empty N",
			key: &Key{
				N: "",
				E: validE,
			},
			expectError: false, // Empty string is valid base64, results in zero
		},
		{
			name: "empty E",
			key: &Key{
				N: validN,
				E: "",
			},
			expectError: false, // Empty string is valid base64, results in zero exponent
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pk, err := tc.key.PublicKey()

			if tc.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				rsaPK, ok := pk.(*rsa.PublicKey)
				if !ok {
					t.Fatalf("expected *rsa.PublicKey, got %T", pk)
				}

				if tc.validate != nil && !tc.validate(rsaPK) {
					t.Fatalf("validation failed for public key: N=%v, E=%v", rsaPK.N, rsaPK.E)
				}
			}
		})
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	validN := base64.RawURLEncoding.EncodeToString(big.NewInt(3233).Bytes())
	validE := base64.RawURLEncoding.EncodeToString(big.NewInt(17).Bytes())

	testCases := []struct {
		name        string
		input       map[string]any
		expected    *Key
		expectError bool
		errorCheck  func(error) bool
	}{
		{
			name:     "nil map",
			input:    nil,
			expected: nil,
		},
		{
			name: "valid RSA key",
			input: map[string]any{
				"kty": "RSA",
				"n":   validN,
				"e":   validE,
			},
			expected: &Key{
				N: validN,
				E: validE,
			},
		},
		{
			name: "wrong kty",
			input: map[string]any{
				"kty": "EC",
				"n":   validN,
				"e":   validE,
			},
			expectError: true,
			errorCheck: func(err error) bool {
				return errors.Is(err, motmedelJwkErrors.ErrKtyMismatch)
			},
		},
		{
			name: "missing kty",
			input: map[string]any{
				"n": validN,
				"e": validE,
			},
			expectError: true,
		},
		{
			name: "missing n",
			input: map[string]any{
				"kty": "RSA",
				"e":   validE,
			},
			expectError: true,
		},
		{
			name: "missing e",
			input: map[string]any{
				"kty": "RSA",
				"n":   validN,
			},
			expectError: true,
		},
		{
			name: "kty wrong type",
			input: map[string]any{
				"kty": 123,
				"n":   validN,
				"e":   validE,
			},
			expectError: true,
		},
		{
			name: "n wrong type",
			input: map[string]any{
				"kty": "RSA",
				"n":   123,
				"e":   validE,
			},
			expectError: true,
		},
		{
			name: "e wrong type",
			input: map[string]any{
				"kty": "RSA",
				"n":   validN,
				"e":   123,
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			key, err := New(tc.input)

			if tc.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if tc.errorCheck != nil && !tc.errorCheck(err) {
					t.Fatalf("error check failed for error: %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if tc.expected == nil {
					if key != nil {
						t.Fatalf("expected nil, got %v", key)
					}
				} else {
					if key == nil {
						t.Fatal("expected non-nil key, got nil")
					}
					if key.N != tc.expected.N {
						t.Fatalf("expected N=%s, got N=%s", tc.expected.N, key.N)
					}
					if key.E != tc.expected.E {
						t.Fatalf("expected E=%s, got E=%s", tc.expected.E, key.E)
					}
				}
			}
		})
	}
}
