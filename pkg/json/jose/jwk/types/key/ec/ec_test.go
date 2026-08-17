package ec

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"math/big"
	"testing"

	altshiftJwkErrors "github.com/altshiftab/utils_go/pkg/json/jose/jwk/errors"
)

func TestCurveFromCrv(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		crv      string
		expected elliptic.Curve
	}{
		{
			name:     "P-256",
			crv:      "P-256",
			expected: elliptic.P256(),
		},
		{
			name:     "P-384",
			crv:      "P-384",
			expected: elliptic.P384(),
		},
		{
			name:     "P-521",
			crv:      "P-521",
			expected: elliptic.P521(),
		},
		{
			name:     "unknown curve",
			crv:      "P-999",
			expected: nil,
		},
		{
			name:     "empty string",
			crv:      "",
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := curveFromCrv(tc.crv)
			if result != tc.expected {
				t.Fatalf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestKey_PublicKey(t *testing.T) {
	t.Parallel()

	// Generate valid EC coordinates for P-256 (using a known test vector)
	// These are arbitrary but valid-looking base64 encoded coordinates
	validX := base64.RawURLEncoding.EncodeToString(make([]byte, 32)) // 32 bytes for P-256
	validY := base64.RawURLEncoding.EncodeToString(make([]byte, 32))

	testCases := []struct {
		name        string
		key         *Key
		expectError bool
		errorCheck  func(error) bool
	}{
		{
			name: "valid P-256 key",
			key: &Key{
				Crv: "P-256",
				X:   validX,
				Y:   validY,
			},
			expectError: false,
		},
		{
			name: "valid P-384 key",
			key: &Key{
				Crv: "P-384",
				X:   base64.RawURLEncoding.EncodeToString(make([]byte, 48)),
				Y:   base64.RawURLEncoding.EncodeToString(make([]byte, 48)),
			},
			expectError: false,
		},
		{
			name: "valid P-521 key",
			key: &Key{
				Crv: "P-521",
				X:   base64.RawURLEncoding.EncodeToString(make([]byte, 66)),
				Y:   base64.RawURLEncoding.EncodeToString(make([]byte, 66)),
			},
			expectError: false,
		},
		{
			name: "unsupported curve",
			key: &Key{
				Crv: "P-999",
				X:   validX,
				Y:   validY,
			},
			expectError: true,
			errorCheck: func(err error) bool {
				return errors.Is(err, altshiftJwkErrors.ErrUnsupportedCrv)
			},
		},
		{
			name: "invalid base64 X",
			key: &Key{
				Crv: "P-256",
				X:   "!!!invalid!!!",
				Y:   validY,
			},
			expectError: true,
		},
		{
			name: "invalid base64 Y",
			key: &Key{
				Crv: "P-256",
				X:   validX,
				Y:   "!!!invalid!!!",
			},
			expectError: true,
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
				if tc.errorCheck != nil && !tc.errorCheck(err) {
					t.Fatalf("error check failed for error: %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				_, ok := pk.(*ecdsa.PublicKey)
				if !ok {
					t.Fatalf("expected *ecdsa.PublicKey, got %T", pk)
				}
			}
		})
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	validX := base64.RawURLEncoding.EncodeToString(big.NewInt(12345).Bytes())
	validY := base64.RawURLEncoding.EncodeToString(big.NewInt(67890).Bytes())

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
			name: "valid EC key",
			input: map[string]any{
				"kty": "EC",
				"crv": "P-256",
				"x":   validX,
				"y":   validY,
			},
			expected: &Key{
				Crv: "P-256",
				X:   validX,
				Y:   validY,
			},
		},
		{
			name: "wrong kty",
			input: map[string]any{
				"kty": "RSA",
				"crv": "P-256",
				"x":   validX,
				"y":   validY,
			},
			expectError: true,
			errorCheck: func(err error) bool {
				return errors.Is(err, altshiftJwkErrors.ErrKtyMismatch)
			},
		},
		{
			name: "missing kty",
			input: map[string]any{
				"crv": "P-256",
				"x":   validX,
				"y":   validY,
			},
			expectError: true,
		},
		{
			name: "missing crv",
			input: map[string]any{
				"kty": "EC",
				"x":   validX,
				"y":   validY,
			},
			expectError: true,
		},
		{
			name: "missing x",
			input: map[string]any{
				"kty": "EC",
				"crv": "P-256",
				"y":   validY,
			},
			expectError: true,
		},
		{
			name: "missing y",
			input: map[string]any{
				"kty": "EC",
				"crv": "P-256",
				"x":   validX,
			},
			expectError: true,
		},
		{
			name: "kty wrong type",
			input: map[string]any{
				"kty": 123,
				"crv": "P-256",
				"x":   validX,
				"y":   validY,
			},
			expectError: true,
		},
		{
			name: "crv wrong type",
			input: map[string]any{
				"kty": "EC",
				"crv": 256,
				"x":   validX,
				"y":   validY,
			},
			expectError: true,
		},
		{
			name: "x wrong type",
			input: map[string]any{
				"kty": "EC",
				"crv": "P-256",
				"x":   123,
				"y":   validY,
			},
			expectError: true,
		},
		{
			name: "y wrong type",
			input: map[string]any{
				"kty": "EC",
				"crv": "P-256",
				"x":   validX,
				"y":   123,
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
					if key.Crv != tc.expected.Crv {
						t.Fatalf("expected Crv=%s, got Crv=%s", tc.expected.Crv, key.Crv)
					}
					if key.X != tc.expected.X {
						t.Fatalf("expected X=%s, got X=%s", tc.expected.X, key.X)
					}
					if key.Y != tc.expected.Y {
						t.Fatalf("expected Y=%s, got Y=%s", tc.expected.Y, key.Y)
					}
				}
			}
		})
	}
}

func TestNewFromPublicKey_RoundTrip(t *testing.T) {
	t.Parallel()

	for _, curve := range []elliptic.Curve{elliptic.P256(), elliptic.P384(), elliptic.P521()} {
		t.Run(curve.Params().Name, func(t *testing.T) {
			t.Parallel()

			priv, err := ecdsa.GenerateKey(curve, rand.Reader)
			if err != nil {
				t.Fatalf("generate: %v", err)
			}

			jwk, err := NewFromPublicKey(&priv.PublicKey)
			if err != nil {
				t.Fatalf("NewFromPublicKey: %v", err)
			}

			// Reconstruct from the JWK and compare — verifies the X/Y encoding.
			pub, err := jwk.PublicKey()
			if err != nil {
				t.Fatalf("PublicKey: %v", err)
			}
			got, ok := pub.(*ecdsa.PublicKey)
			if !ok {
				t.Fatalf("PublicKey type = %T, want *ecdsa.PublicKey", pub)
			}
			if !got.Equal(&priv.PublicKey) {
				t.Fatal("round-tripped public key does not match original")
			}
		})
	}
}

func TestNewFromPublicKey_Nil(t *testing.T) {
	t.Parallel()

	key, err := NewFromPublicKey(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != nil {
		t.Fatalf("got %v, want nil", key)
	}
}

func TestNewFromPublicKey_UnsupportedCurve(t *testing.T) {
	t.Parallel()

	priv, err := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := NewFromPublicKey(&priv.PublicKey); !errors.Is(err, altshiftJwkErrors.ErrUnsupportedCrv) {
		t.Fatalf("err = %v, want ErrUnsupportedCrv", err)
	}
}
