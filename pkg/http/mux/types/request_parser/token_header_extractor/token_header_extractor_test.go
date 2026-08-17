package token_header_extractor

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/token_header_extractor/token_header_extractor_config"
)

func TestParse(t *testing.T) {
	t.Parallel()

	t.Run("strips the value prefix", func(t *testing.T) {
		t.Parallel()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		request.Header.Set("Authorization", "Bearer secret-token")
		value, responseError := New().Parse(request)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if value != "secret-token" {
			t.Fatalf("got %q, want secret-token", value)
		}
	})

	t.Run("missing header defaults to 401", func(t *testing.T) {
		t.Parallel()
		_, responseError := New().Parse(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
		if responseError == nil || responseError.ProblemDetail == nil || responseError.ProblemDetail.Status != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %#v", responseError)
		}
	})

	t.Run("custom header name and prefix", func(t *testing.T) {
		t.Parallel()
		parser := New(
			token_header_extractor_config.WithHeaderName("X-Token"),
			token_header_extractor_config.WithHeaderValuePrefix("Token "),
		)
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		request.Header.Set("X-Token", "Token abc")
		value, responseError := parser.Parse(request)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if value != "abc" {
			t.Fatalf("got %q, want abc", value)
		}
	})
}
