package metadata_token_source

import (
	"context"
	"encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/token_response"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return u
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("nil metadata base url", func(t *testing.T) {
		t.Parallel()

		ts, err := New(context.Background(), nil, nil)
		if err == nil {
			t.Fatal("expected error for nil metadata base url")
		}
		if ts != nil {
			t.Errorf("expected nil token source on error, got %v", ts)
		}
		ne, ok := errors.AsType[*nil_error.Error](err)
		if !ok {
			t.Fatalf("err type = %T (%v), want *nil_error.Error", err, err)
		}
		if ne.Field != "metadata base url" {
			t.Errorf("Field = %q, want %q", ne.Field, "metadata base url")
		}
	})

	t.Run("fields set", func(t *testing.T) {
		t.Parallel()

		baseUrl := mustParseURL(t, "http://metadata.internal")
		scopes := []string{"a", "b"}
		options := []fetch_config.Option{fetch_config.WithMethod(http.MethodGet)}

		ts, err := New(context.Background(), baseUrl, scopes, options...)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ts.metadataBaseUrl != baseUrl {
			t.Errorf("metadataBaseUrl mismatch")
		}
		if len(ts.scopes) != len(scopes) {
			t.Errorf("scopes len = %d, want %d", len(ts.scopes), len(scopes))
		}
		if len(ts.options) != len(options) {
			t.Errorf("options len = %d, want %d", len(ts.options), len(options))
		}
	})
}

func TestToken(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		scopes     []string
		wantScopes string // expected "scopes" query value; "" means absent
	}{
		{name: "no scopes", scopes: nil, wantScopes: ""},
		{name: "with scopes", scopes: []string{"scope-a", "scope-b"}, wantScopes: "scope-a,scope-b"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/instance/service-accounts/default/token" {
					t.Errorf("path = %q, want %q", r.URL.Path, "/instance/service-accounts/default/token")
				}
				if got := r.Header.Get("Metadata-Flavor"); got != "Google" {
					t.Errorf("Metadata-Flavor = %q, want %q", got, "Google")
				}
				if got := r.URL.Query().Get("scopes"); got != testCase.wantScopes {
					t.Errorf("scopes query = %q, want %q", got, testCase.wantScopes)
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.MarshalWrite(w, &token_response.Response{ //nolint:gosec // G117: fake OAuth token in test fixture
					AccessToken: "access-token",
					TokenType:   "Bearer",
					ExpiresIn:   3600,
				}); err != nil {
					t.Errorf("encode: %v", err)
				}
			}))
			t.Cleanup(server.Close)

			ts, err := New(context.Background(), mustParseURL(t, server.URL), testCase.scopes)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			tok, err := ts.Token()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tok.AccessToken != "access-token" {
				t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "access-token")
			}
			if tok.Type() != "Bearer" {
				t.Errorf("Type() = %q, want %q", tok.Type(), "Bearer")
			}
			if tok.Expiry.IsZero() {
				t.Error("expected non-zero Expiry")
			}
		})
	}
}

func TestToken_CancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ts, err := New(ctx, mustParseURL(t, "http://metadata.internal"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := ts.Token(); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestToken_ServerError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	ts, err := New(context.Background(), mustParseURL(t, server.URL), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := ts.Token(); err == nil {
		t.Fatal("expected error for non-2xx status")
	}
}

func TestToken_EmptyBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	ts, err := New(context.Background(), mustParseURL(t, server.URL), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = ts.Token()
	if err == nil {
		t.Fatal("expected error for empty token response")
	}
	ne, ok := errors.AsType[*nil_error.Error](err)
	if !ok {
		t.Fatalf("err type = %T (%v), want *nil_error.Error", err, err)
	}
	if ne.Field != "token response" {
		t.Errorf("Field = %q, want %q", ne.Field, "token response")
	}
}
