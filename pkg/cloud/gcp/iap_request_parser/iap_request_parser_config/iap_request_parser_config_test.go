package iap_request_parser_config

import (
	"net/url"
	"slices"
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()

	jwkUrl := &url.URL{Scheme: "https", Host: "keys.example.test"}

	// No options is not a usable parser: New refuses without an audience, because without one it
	// would admit an assertion minted for any service IAP fronts. Only the key url has an answer.
	empty := New()
	if empty == nil {
		t.Fatal("expected a config, got nil")
	}
	if empty.Audience != "" || empty.JwkUrl != nil {
		t.Errorf("expected everything unset, got %+v", empty)
	}
	// Naming nobody admits whoever IAP itself admitted, which is the ordinary case.
	if len(empty.AllowedEmails) != 0 || len(empty.AllowedHostedDomains) != 0 {
		t.Errorf("expected no account list, got %+v", empty)
	}

	// A nil option is skipped rather than panicking, so a caller can pass one conditionally.
	if New(nil) == nil {
		t.Error("expected a nil option to be skipped")
	}

	config := New(
		WithAudience("/projects/1/global/backendServices/2"),
		WithAllowedEmails("a@example.test"),
		// Accumulating rather than replacing: naming a domain and an address outside it is how a
		// deployment says "everyone here, and this one guest", and replacing would drop one half.
		WithAllowedEmails("b@example.test"),
		WithAllowedHostedDomains("example.test"),
		WithAllowedHostedDomains("other.test"),
		WithJwkUrl(jwkUrl),
	)

	if config.Audience != "/projects/1/global/backendServices/2" {
		t.Errorf("expected the audience to be taken, got %q", config.Audience)
	}
	if !slices.Equal(config.AllowedEmails, []string{"a@example.test", "b@example.test"}) {
		t.Errorf("expected the addresses to accumulate, got %v", config.AllowedEmails)
	}
	if !slices.Equal(config.AllowedHostedDomains, []string{"example.test", "other.test"}) {
		t.Errorf("expected the domains to accumulate, got %v", config.AllowedHostedDomains)
	}
	if config.JwkUrl != jwkUrl {
		t.Errorf("expected the key url to be taken, got %v", config.JwkUrl)
	}
}
