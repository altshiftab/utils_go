package id_token_request_parser_config

import (
	"net/url"
	"slices"
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()

	jwkUrl := &url.URL{Scheme: "https", Host: "keys.example.test"}

	// No options is not a usable parser: an audience and a service account have no safe guess, and
	// New refuses rather than defaulting either. Only the key url has an answer.
	empty := New()
	if empty == nil {
		t.Fatal("expected a config, got nil")
	}
	if empty.Audience != "" || len(empty.ServiceAccountEmails) != 0 || empty.JwkUrl != nil {
		t.Errorf("expected everything unset, got %+v", empty)
	}

	// A nil option is skipped rather than panicking, so a caller can pass one conditionally.
	if New(nil) == nil {
		t.Error("expected a nil option to be skipped")
	}

	config := New(
		WithAudience("https://monitor.example.test/api/fetch/assets"),
		WithServiceAccountEmails("a@example.test"),
		// Accumulating rather than replacing: a service called by two Google products names both,
		// and replacing would silently admit only the last.
		WithServiceAccountEmails("b@example.test", "c@example.test"),
		WithJwkUrl(jwkUrl),
	)

	if config.Audience != "https://monitor.example.test/api/fetch/assets" {
		t.Errorf("expected the audience to be taken, got %q", config.Audience)
	}
	if !slices.Equal(config.ServiceAccountEmails, []string{"a@example.test", "b@example.test", "c@example.test"}) {
		t.Errorf("expected the accounts to accumulate, got %v", config.ServiceAccountEmails)
	}
	if config.JwkUrl != jwkUrl {
		t.Errorf("expected the key url to be taken, got %v", config.JwkUrl)
	}
}
