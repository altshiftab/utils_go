package id_token_request_parser

import (
	"net/url"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/id_token_request_parser/id_token_request_parser_config"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/numeric_date"
)

const (
	testAudience    = "https://monitor.example.com/api/fetch/leaks"
	testCallerEmail = "signals-monitor-scheduler@example.iam.gserviceaccount.com"
)

// validClaims is a token that should get in. Each test case below spoils exactly one thing about
// it, so what the case is about is the difference from this.
func validClaims() map[string]any {
	return map[string]any{
		"exp":            numeric_date.Date{Time: time.Now().Add(time.Hour)},
		"iss":            "https://accounts.google.com",
		"aud":            testAudience,
		"email":          testCallerEmail,
		"email_verified": true,
	}
}

func TestMakeClaimsValidator(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		// spoil mutates the otherwise valid claims into what the case is about.
		spoil       func(claims map[string]any)
		expectError bool
	}{
		{name: "a valid token gets in", spoil: func(map[string]any) {}},
		{
			name: "the other spelling of the issuer is accepted",
			spoil: func(claims map[string]any) {
				claims["iss"] = "accounts.google.com"
			},
		},
		{
			// Cloud Scheduler can mint a token with several audiences; ours has to be among them.
			name: "an audience list containing ours is accepted",
			spoil: func(claims map[string]any) {
				claims["aud"] = []any{"https://elsewhere.example.com", testAudience}
			},
		},
		{
			name:        "an expired token is refused",
			spoil:       func(claims map[string]any) { claims["exp"] = numeric_date.Date{Time: time.Now().Add(-time.Hour)} },
			expectError: true,
		},
		{
			// Anyone can mint a Google token; only Google's issuer is trusted, and only for us.
			name:        "another issuer is refused",
			spoil:       func(claims map[string]any) { claims["iss"] = "https://accounts.evil.example" },
			expectError: true,
		},
		{
			// A token minted for another service must not be replayable against this one.
			name:        "another audience is refused",
			spoil:       func(claims map[string]any) { claims["aud"] = "https://elsewhere.example.com" },
			expectError: true,
		},
		{
			// The endpoint is reachable by anyone, so the caller's identity is what gates it.
			name:        "another caller is refused",
			spoil:       func(claims map[string]any) { claims["email"] = "someone-else@example.com" },
			expectError: true,
		},
		{
			name:        "an unverified email is refused",
			spoil:       func(claims map[string]any) { claims["email_verified"] = false },
			expectError: true,
		},
		{
			name:        "a token with no expiry is refused",
			spoil:       func(claims map[string]any) { delete(claims, "exp") },
			expectError: true,
		},
		{
			name:        "a token with no audience is refused",
			spoil:       func(claims map[string]any) { delete(claims, "aud") },
			expectError: true,
		},
		{
			name:        "a token with no email is refused",
			spoil:       func(claims map[string]any) { delete(claims, "email") },
			expectError: true,
		},
		{
			name:        "a token with no issuer is refused",
			spoil:       func(claims map[string]any) { delete(claims, "iss") },
			expectError: true,
		},
	}

	validator := MakeClaimsValidator(testAudience, []string{testCallerEmail})

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			claims := validClaims()
			testCase.spoil(claims)

			err := validator.Validate(claims)

			if testCase.expectError {
				if err == nil {
					t.Fatalf("%s: expected the token to be refused, it was accepted", testCase.name)
				}
				return
			}
			if err != nil {
				t.Errorf("%s: expected the token to be accepted, got %v", testCase.name, err)
			}
		})
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	jwkUrl, err := JwkUrl()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	testCases := []struct {
		name        string
		audience    string
		callerEmail string
		jwkUrl      *url.URL
		expectError bool
	}{
		{
			name:        "an audience, a caller and a key url are enough",
			audience:    testAudience,
			callerEmail: testCallerEmail,
			jwkUrl:      jwkUrl,
		},
		{
			name:        "an empty audience is an error",
			callerEmail: testCallerEmail,
			jwkUrl:      jwkUrl,
			expectError: true,
		},
		{
			name:        "an empty caller is an error",
			audience:    testAudience,
			jwkUrl:      jwkUrl,
			expectError: true,
		},
		{
			// Not an error any more: Google's keys are where a Google token is verified against,
			// so a caller that says nothing means those.
			name:        "no key url falls back to Google's",
			audience:    testAudience,
			callerEmail: testCallerEmail,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			parser, err := New(
				id_token_request_parser_config.WithAudience(testCase.audience),
				id_token_request_parser_config.WithServiceAccountEmails(testCase.callerEmail),
				id_token_request_parser_config.WithJwkUrl(testCase.jwkUrl),
			)

			if testCase.expectError {
				if err == nil {
					t.Fatalf("%s: expected an error, got none", testCase.name)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", testCase.name, err)
			}
			if parser == nil {
				t.Errorf("%s: expected a parser, got nil", testCase.name)
			}
		})
	}
}

// TestNewDefaultsToGooglesKeys holds what a caller that names no key url gets. It is the only
// default here: an audience and a service account have no safe guess, and defaulting either would
// admit far more than a caller meant.
func TestNewDefaultsToGooglesKeys(t *testing.T) {
	t.Parallel()

	if _, err := New(
		id_token_request_parser_config.WithAudience(testAudience),
		id_token_request_parser_config.WithServiceAccountEmails(testCallerEmail),
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jwkUrl, err := JwkUrl()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jwkUrl.String() != JwkUrlString {
		t.Errorf("expected %q, got %q", JwkUrlString, jwkUrl)
	}
}

// TestNewAdmitsSeveralServiceAccounts holds why the option is variadic: a service is commonly
// called by more than one Google product, and racing two parsers that differ only in an address
// would be a strange way to say so.
func TestNewAdmitsSeveralServiceAccounts(t *testing.T) {
	t.Parallel()

	parser, err := New(
		id_token_request_parser_config.WithAudience(testAudience),
		id_token_request_parser_config.WithServiceAccountEmails("a@example.test"),
		id_token_request_parser_config.WithServiceAccountEmails("b@example.test"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parser == nil {
		t.Error("expected a parser, got nil")
	}

	// Both are admitted, and nothing else is.
	validate := MakeClaimsValidator(testAudience, []string{"a@example.test", "b@example.test"})

	for _, email := range []string{"a@example.test", "b@example.test"} {
		claims := validClaims()
		claims["email"] = email

		if err := validate.Validate(claims); err != nil {
			t.Errorf("expected %s to be admitted, got %v", email, err)
		}
	}

	unnamed := validClaims()
	unnamed["email"] = "c@example.test"
	if err := validate.Validate(unnamed); err == nil {
		t.Error("expected an account that was not named to be refused")
	}
}

func TestJwkUrl(t *testing.T) {
	t.Parallel()

	jwkUrl, err := JwkUrl()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if jwkUrl.String() != JwkUrlString {
		t.Errorf("expected %q, got %q", JwkUrlString, jwkUrl.String())
	}
	// The keys are fetched over the network, and fetching them over anything but TLS would let the
	// signatures be chosen by whoever is in the way.
	if jwkUrl.Scheme != "https" {
		t.Errorf("expected the keys to be fetched over https, got %q", jwkUrl.Scheme)
	}
}
