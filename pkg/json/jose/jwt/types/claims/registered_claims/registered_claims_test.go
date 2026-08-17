package registered_claims

import (
	"encoding/json/v2"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/claim_strings"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/numeric_date"
)

// referenceTime is a fixed reference instant so time-based tests are deterministic.
var referenceTime = time.Unix(1700000000, 0).UTC()

func TestNew(t *testing.T) {
	t.Parallel()

	exp := float64(referenceTime.Add(time.Hour).Unix())
	nbf := float64(referenceTime.Add(-time.Hour).Unix())
	iat := float64(referenceTime.Unix())

	testCases := []struct {
		name      string
		input     map[string]any
		expectNil bool
		expectErr bool
		check     func(t *testing.T, claims *Claims)
	}{
		{
			name:      "empty map returns nil, nil",
			input:     map[string]any{},
			expectNil: true,
		},
		{
			name:      "nil map returns nil, nil",
			input:     nil,
			expectNil: true,
		},
		{
			name: "all fields populated",
			input: map[string]any{
				"iss": "issuer",
				"sub": "subject",
				"aud": []any{"aud1", "aud2"},
				"exp": exp,
				"nbf": nbf,
				"iat": iat,
				"jti": "token-id",
			},
			check: func(t *testing.T, claims *Claims) {
				if claims.Issuer != "issuer" {
					t.Errorf("Issuer = %q, want %q", claims.Issuer, "issuer")
				}
				if claims.Subject != "subject" {
					t.Errorf("Subject = %q, want %q", claims.Subject, "subject")
				}
				if len(claims.Audience) != 2 || claims.Audience[0] != "aud1" || claims.Audience[1] != "aud2" {
					t.Errorf("Audience = %v, want [aud1 aud2]", claims.Audience)
				}
				if claims.ExpiresAt == nil || claims.ExpiresAt.Unix() != int64(exp) {
					t.Errorf("ExpiresAt = %v, want unix %d", claims.ExpiresAt, int64(exp))
				}
				if claims.NotBefore == nil || claims.NotBefore.Unix() != int64(nbf) {
					t.Errorf("NotBefore = %v, want unix %d", claims.NotBefore, int64(nbf))
				}
				if claims.IssuedAt == nil || claims.IssuedAt.Unix() != int64(iat) {
					t.Errorf("IssuedAt = %v, want unix %d", claims.IssuedAt, int64(iat))
				}
				if claims.Id != "token-id" {
					t.Errorf("Id = %q, want %q", claims.Id, "token-id")
				}
			},
		},
		{
			name: "single string audience",
			input: map[string]any{
				"aud": "solo",
			},
			check: func(t *testing.T, claims *Claims) {
				if len(claims.Audience) != 1 || claims.Audience[0] != "solo" {
					t.Errorf("Audience = %v, want [solo]", claims.Audience)
				}
			},
		},
		{
			name: "explicit nil exp yields nil ExpiresAt",
			input: map[string]any{
				"iss": "x",
				"exp": nil,
			},
			check: func(t *testing.T, claims *Claims) {
				if claims.ExpiresAt != nil {
					t.Errorf("ExpiresAt = %v, want nil", claims.ExpiresAt)
				}
			},
		},
		{
			name: "zero exp yields nil ExpiresAt without error",
			input: map[string]any{
				"iss": "x",
				"exp": float64(0),
			},
			check: func(t *testing.T, claims *Claims) {
				if claims.ExpiresAt != nil {
					t.Errorf("ExpiresAt = %v, want nil", claims.ExpiresAt)
				}
			},
		},
		{
			name: "invalid iss type errors",
			input: map[string]any{
				"iss": 123,
			},
			expectErr: true,
		},
		{
			name: "invalid exp type errors",
			input: map[string]any{
				"exp": "not-a-number",
			},
			expectErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			claims, err := New(testCase.input)

			if testCase.expectErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if testCase.expectNil {
				if claims != nil {
					t.Fatalf("expected nil claims, got %v", claims)
				}
				return
			}

			if claims == nil {
				t.Fatal("expected non-nil claims")
			}
			if testCase.check != nil {
				testCase.check(t, claims)
			}
		})
	}
}

func TestJSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := &Claims{
		Issuer:    "issuer",
		Subject:   "subject",
		Audience:  claim_strings.ClaimStrings{"aud1", "aud2"},
		ExpiresAt: numeric_date.New(referenceTime.Add(time.Hour)),
		NotBefore: numeric_date.New(referenceTime.Add(-time.Hour)),
		IssuedAt:  numeric_date.New(referenceTime),
		Id:        "token-id",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Claims
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Issuer != original.Issuer {
		t.Errorf("Issuer = %q, want %q", decoded.Issuer, original.Issuer)
	}
	if decoded.Subject != original.Subject {
		t.Errorf("Subject = %q, want %q", decoded.Subject, original.Subject)
	}
	if len(decoded.Audience) != 2 || decoded.Audience[0] != "aud1" || decoded.Audience[1] != "aud2" {
		t.Errorf("Audience = %v, want [aud1 aud2]", decoded.Audience)
	}
	if decoded.ExpiresAt == nil || decoded.ExpiresAt.Unix() != original.ExpiresAt.Unix() {
		t.Errorf("ExpiresAt = %v, want %v", decoded.ExpiresAt, original.ExpiresAt)
	}
	if decoded.NotBefore == nil || decoded.NotBefore.Unix() != original.NotBefore.Unix() {
		t.Errorf("NotBefore = %v, want %v", decoded.NotBefore, original.NotBefore)
	}
	if decoded.IssuedAt == nil || decoded.IssuedAt.Unix() != original.IssuedAt.Unix() {
		t.Errorf("IssuedAt = %v, want %v", decoded.IssuedAt, original.IssuedAt)
	}
	if decoded.Id != original.Id {
		t.Errorf("Id = %q, want %q", decoded.Id, original.Id)
	}
}

func TestJSONOmitsEmpty(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(&Claims{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != "{}" {
		t.Errorf("marshal empty Claims = %s, want {}", data)
	}
}

func TestNewParsedClaims(t *testing.T) {
	t.Parallel()

	exp := float64(referenceTime.Add(time.Hour).Unix())

	testCases := []struct {
		name      string
		input     map[string]any
		expectNil bool
		expectErr bool
		check     func(t *testing.T, parsed ParsedClaims)
	}{
		{
			name:      "nil map returns nil, nil",
			input:     nil,
			expectNil: true,
		},
		{
			name: "converts exp to Date and aud to ClaimStrings",
			input: map[string]any{
				"iss": "issuer",
				"exp": exp,
				"aud": "solo",
			},
			check: func(t *testing.T, parsed ParsedClaims) {
				expDate, ok := parsed["exp"].(numeric_date.Date)
				if !ok {
					t.Fatalf("exp = %T, want numeric_date.Date", parsed["exp"])
				}
				if expDate.Unix() != int64(exp) {
					t.Errorf("exp unix = %d, want %d", expDate.Unix(), int64(exp))
				}
				aud, ok := parsed["aud"].(claim_strings.ClaimStrings)
				if !ok {
					t.Fatalf("aud = %T, want claim_strings.ClaimStrings", parsed["aud"])
				}
				if len(aud) != 1 || aud[0] != "solo" {
					t.Errorf("aud = %v, want [solo]", aud)
				}
				if parsed["iss"] != "issuer" {
					t.Errorf("iss = %v, want issuer", parsed["iss"])
				}
			},
		},
		{
			name: "zero exp errors (numeric date nil)",
			input: map[string]any{
				"exp": float64(0),
			},
			expectErr: true,
		},
		{
			name: "invalid aud type errors",
			input: map[string]any{
				"aud": 42,
			},
			expectErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := NewParsedClaims(testCase.input)

			if testCase.expectErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if testCase.expectNil {
				if parsed != nil {
					t.Fatalf("expected nil parsed claims, got %v", parsed)
				}
				return
			}

			if testCase.check != nil {
				testCase.check(t, parsed)
			}
		})
	}
}

func TestNewParsedClaimsDoesNotMutateInput(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"exp": float64(referenceTime.Unix()),
	}
	if _, err := NewParsedClaims(input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := input["exp"].(float64); !ok {
		t.Errorf("input exp mutated to %T, want float64", input["exp"])
	}
}
