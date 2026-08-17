package token_cookie_extractor

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/token_cookie_extractor/token_cookie_extractor_config"
)

func TestParse(t *testing.T) {
	t.Parallel()

	t.Run("present cookie", func(t *testing.T) {
		t.Parallel()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		request.AddCookie(&http.Cookie{Name: "session", Value: "tok", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		value, responseError := New().Parse(request)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if value != "tok" {
			t.Fatalf("got %q, want tok", value)
		}
	})

	t.Run("missing cookie defaults to 401", func(t *testing.T) {
		t.Parallel()
		_, responseError := New().Parse(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
		if responseError == nil || responseError.ProblemDetail == nil || responseError.ProblemDetail.Status != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %#v", responseError)
		}
	})

	t.Run("empty name is a server error", func(t *testing.T) {
		t.Parallel()
		parser := New(token_cookie_extractor_config.WithName(""))
		_, responseError := parser.Parse(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})
}
