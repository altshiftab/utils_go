package session_claims

import (
	"encoding/json/v2"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/claims/registered_claims"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/numeric_date"
)

// referenceTime is a fixed reference instant so time-based tests are deterministic.
var referenceTime = time.Unix(1700000000, 0).UTC()

func TestNew(t *testing.T) {
	t.Parallel()

	authTime := float64(referenceTime.Unix())

	testCases := []struct {
		name      string
		input     map[string]any
		expectErr bool
		check     func(t *testing.T, claims *Claims)
	}{
		{
			name:      "empty map errors (nil registered claims)",
			input:     map[string]any{},
			expectErr: true,
		},
		{
			name:      "nil map errors (nil registered claims)",
			input:     nil,
			expectErr: true,
		},
		{
			name: "all session fields populated",
			input: map[string]any{
				"sub":       "subject",
				"amr":       []any{"pwd", "mfa"},
				"auth_time": authTime,
				"azp":       "client-id",
				"roles":     []any{"admin", "user"},
			},
			check: func(t *testing.T, claims *Claims) {
				if claims.Subject != "subject" {
					t.Errorf("Subject = %q, want %q", claims.Subject, "subject")
				}
				if len(claims.AuthenticationMethods) != 2 ||
					claims.AuthenticationMethods[0] != "pwd" ||
					claims.AuthenticationMethods[1] != "mfa" {
					t.Errorf("AuthenticationMethods = %v, want [pwd mfa]", claims.AuthenticationMethods)
				}
				if claims.AuthenticatedAt == nil || claims.AuthenticatedAt.Unix() != int64(authTime) {
					t.Errorf("AuthenticatedAt = %v, want unix %d", claims.AuthenticatedAt, int64(authTime))
				}
				if claims.AuthorizedParty != "client-id" {
					t.Errorf("AuthorizedParty = %q, want %q", claims.AuthorizedParty, "client-id")
				}
				if len(claims.Roles) != 2 || claims.Roles[0] != "admin" || claims.Roles[1] != "user" {
					t.Errorf("Roles = %v, want [admin user]", claims.Roles)
				}
			},
		},
		{
			name: "only registered field yields empty session fields",
			input: map[string]any{
				"iss": "issuer",
			},
			check: func(t *testing.T, claims *Claims) {
				if claims.Issuer != "issuer" {
					t.Errorf("Issuer = %q, want %q", claims.Issuer, "issuer")
				}
				if claims.AuthenticationMethods != nil {
					t.Errorf("AuthenticationMethods = %v, want nil", claims.AuthenticationMethods)
				}
			},
		},
		{
			name: "invalid amr type errors",
			input: map[string]any{
				"sub": "s",
				"amr": 123,
			},
			expectErr: true,
		},
		{
			name: "invalid azp type errors",
			input: map[string]any{
				"sub": "s",
				"azp": 123,
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
		Claims: registered_claims.Claims{
			Issuer:   "issuer",
			Subject:  "subject",
			IssuedAt: numeric_date.New(referenceTime),
		},
		AuthenticationMethods: []string{"pwd", "mfa"},
		AuthenticatedAt:       numeric_date.New(referenceTime.Add(-time.Minute)),
		AuthorizedParty:       "client-id",
		Roles:                 []string{"admin"},
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
	if decoded.IssuedAt == nil || decoded.IssuedAt.Unix() != original.IssuedAt.Unix() {
		t.Errorf("IssuedAt = %v, want %v", decoded.IssuedAt, original.IssuedAt)
	}
	if len(decoded.AuthenticationMethods) != 2 ||
		decoded.AuthenticationMethods[0] != "pwd" ||
		decoded.AuthenticationMethods[1] != "mfa" {
		t.Errorf("AuthenticationMethods = %v, want [pwd mfa]", decoded.AuthenticationMethods)
	}
	if decoded.AuthenticatedAt == nil || decoded.AuthenticatedAt.Unix() != original.AuthenticatedAt.Unix() {
		t.Errorf("AuthenticatedAt = %v, want %v", decoded.AuthenticatedAt, original.AuthenticatedAt)
	}
	if decoded.AuthorizedParty != original.AuthorizedParty {
		t.Errorf("AuthorizedParty = %q, want %q", decoded.AuthorizedParty, original.AuthorizedParty)
	}
	if len(decoded.Roles) != 1 || decoded.Roles[0] != "admin" {
		t.Errorf("Roles = %v, want [admin]", decoded.Roles)
	}
}

func TestNewParsedClaims(t *testing.T) {
	t.Parallel()

	authTime := float64(referenceTime.Unix())

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
			name: "converts auth_time to Date",
			input: map[string]any{
				"sub":       "subject",
				"auth_time": authTime,
			},
			check: func(t *testing.T, parsed ParsedClaims) {
				authDate, ok := parsed["auth_time"].(numeric_date.Date)
				if !ok {
					t.Fatalf("auth_time = %T, want numeric_date.Date", parsed["auth_time"])
				}
				if authDate.Unix() != int64(authTime) {
					t.Errorf("auth_time unix = %d, want %d", authDate.Unix(), int64(authTime))
				}
			},
		},
		{
			name: "zero auth_time errors",
			input: map[string]any{
				"auth_time": float64(0),
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
