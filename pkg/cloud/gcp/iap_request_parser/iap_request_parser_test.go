package iap_request_parser

import (
	"net/url"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/iap_request_parser/iap_request_parser_config"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/numeric_date"
)

const (
	testAudience = "/projects/123456789/global/backendServices/987654321"
	testEmail    = "someone@example.com"
)

// validClaims is an assertion that should get in. Each case below spoils exactly one thing about
// it, so what the case is about is the difference from this.
func validClaims() map[string]any {
	return map[string]any{
		"exp":   numeric_date.Date{Time: time.Now().Add(time.Hour)},
		"iss":   Issuer,
		"aud":   testAudience,
		"sub":   "accounts.google.com:105000000000000000000",
		"email": testEmail,
		"hd":    "example.com",
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
		{name: "a valid assertion"},
		{
			name:        "one that has run out",
			spoil:       func(c map[string]any) { c["exp"] = numeric_date.Date{Time: time.Now().Add(-time.Hour)} },
			expectError: true,
		},
		{
			// The issuer is what says IAP minted it rather than something else Google signs.
			name:        "one Google signed but IAP did not issue",
			spoil:       func(c map[string]any) { c["iss"] = "https://accounts.google.com" },
			expectError: true,
		},
		{
			// The case that matters most: every deployment behind IAP is issued assertions by the
			// same IAP, signed with the same keys, so without the audience an assertion minted for
			// anyone else's service would be accepted here.
			name:        "one minted for another service behind IAP",
			spoil:       func(c map[string]any) { c["aud"] = "/projects/123456789/global/backendServices/111" },
			expectError: true,
		},
		{
			name:        "one identifying nobody",
			spoil:       func(c map[string]any) { c["sub"] = "" },
			expectError: true,
		},
		{
			name:        "one with no subject at all",
			spoil:       func(c map[string]any) { delete(c, "sub") },
			expectError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			claims := validClaims()
			if testCase.spoil != nil {
				testCase.spoil(claims)
			}

			err := MakeClaimsValidator(testAudience, nil, nil).Validate(claims)

			if testCase.expectError && err == nil {
				t.Errorf("%s: expected an error, got none", testCase.name)
			}
			if !testCase.expectError && err != nil {
				t.Errorf("%s: unexpected error: %v", testCase.name, err)
			}
		})
	}
}

// TestNamingNobodyAdmitsWhoeverIapDid holds the ordinary case. IAP has an access policy of its own,
// and repeating it here would mean maintaining it twice and having the two disagree.
func TestNamingNobodyAdmitsWhoeverIapDid(t *testing.T) {
	t.Parallel()

	claims := validClaims()
	claims["email"] = "anyone@wherever.test"
	delete(claims, "hd")

	if err := MakeClaimsValidator(testAudience, nil, nil).Validate(claims); err != nil {
		t.Errorf("expected whoever IAP admitted to be admitted, got %v", err)
	}
}

func TestAllowedAccounts(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		emails         []string
		hostedDomains  []string
		claimEmail     string
		claimHd        string
		noHd           bool
		expectAdmitted bool
	}{
		{
			name:           "an account named outright",
			emails:         []string{testEmail},
			claimEmail:     testEmail,
			expectAdmitted: true,
		},
		{
			// Addresses are folded, so one written in another case still matches.
			name:           "the same account in another case",
			emails:         []string{"Someone@Example.COM"},
			claimEmail:     testEmail,
			expectAdmitted: true,
		},
		{
			name:           "an account in a named domain",
			hostedDomains:  []string{"example.com"},
			claimEmail:     testEmail,
			claimHd:        "example.com",
			expectAdmitted: true,
		},
		{
			// Naming a domain and an address outside it is how a deployment says "everyone here,
			// and this one guest".
			name:           "a guest outside the named domain",
			emails:         []string{"guest@elsewhere.test"},
			hostedDomains:  []string{"example.com"},
			claimEmail:     "guest@elsewhere.test",
			noHd:           true,
			expectAdmitted: true,
		},
		{
			name:          "an account in another domain",
			hostedDomains: []string{"example.com"},
			claimEmail:    "someone@elsewhere.test",
			claimHd:       "elsewhere.test",
		},
		{
			// A consumer account carries no hd claim at all, so naming a domain turns it away
			// rather than admitting it for want of a claim to check.
			name:          "a personal account with no hosted domain",
			hostedDomains: []string{"example.com"},
			claimEmail:    "someone@gmail.com",
			noHd:          true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			claims := validClaims()
			claims["email"] = testCase.claimEmail
			if testCase.noHd {
				delete(claims, "hd")
			} else if testCase.claimHd != "" {
				claims["hd"] = testCase.claimHd
			}

			err := MakeClaimsValidator(
				testAudience,
				lowered(testCase.emails),
				lowered(testCase.hostedDomains),
			).Validate(claims)

			if testCase.expectAdmitted && err != nil {
				t.Errorf("%s: expected admitted, got %v", testCase.name, err)
			}
			if !testCase.expectAdmitted && err == nil {
				t.Errorf("%s: expected refused, got none", testCase.name)
			}
		})
	}
}

func TestAudienceHelpers(t *testing.T) {
	t.Parallel()

	// The project number is the numeric one: IAP mints the assertion with what the resource is
	// called internally, and an audience built from the project id never matches.
	if got := BackendServiceAudience("123456789", "987654321"); got != testAudience {
		t.Errorf("expected %q, got %q", testAudience, got)
	}
	if got := AppEngineAudience("123456789", "my-project"); got != "/projects/123456789/apps/my-project" {
		t.Errorf("unexpected app engine audience %q", got)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	// Without an audience this would admit an assertion minted for any service IAP fronts, which
	// is every service behind IAP anywhere.
	if _, err := New(); err == nil {
		t.Error("expected a missing audience to be an error")
	}

	parser, err := New(iap_request_parser_config.WithAudience(testAudience))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parser == nil {
		t.Error("expected a parser, got nil")
	}

	// A caller that names no key url gets IAP's, which are elliptic curve keys published somewhere
	// other than the ones Google signs ID tokens with.
	jwkUrl, err := JwkUrl()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jwkUrl.String() != JwkUrlString {
		t.Errorf("expected %q, got %q", JwkUrlString, jwkUrl)
	}
	if jwkUrl.Host == "www.googleapis.com" {
		t.Error("expected IAP's keys rather than the ID token ones")
	}

	if _, err := New(
		iap_request_parser_config.WithAudience(testAudience),
		iap_request_parser_config.WithJwkUrl(&url.URL{Scheme: "https", Host: "keys.example.test"}),
	); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
