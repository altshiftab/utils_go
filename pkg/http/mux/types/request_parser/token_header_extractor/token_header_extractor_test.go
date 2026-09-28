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

func TestParse_Challenge(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		options       []token_header_extractor_config.Option
		headerName    string
		headerValues  []string
		wantChallenge string
	}{
		{name: "missing bearer header", wantChallenge: "Bearer"},
		{
			name:          "multiple bearer headers",
			headerName:    "Authorization",
			headerValues:  []string{"Bearer a", "Bearer b"},
			wantChallenge: `Bearer error="invalid_request", error_description="Multiple token headers values."`,
		},
		{
			name:    "a header other than Authorization has no challenge",
			options: []token_header_extractor_config.Option{token_header_extractor_config.WithHeaderName("X-Token")},
		},
		{
			name:    "a prefix other than Bearer has no challenge",
			options: []token_header_extractor_config.Option{token_header_extractor_config.WithHeaderValuePrefix("Token ")},
		},
		{
			name:    "a refusal that is not 401 has no challenge",
			options: []token_header_extractor_config.Option{token_header_extractor_config.WithProblemDetailStatusCode(http.StatusBadRequest)},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			for _, value := range testCase.headerValues {
				request.Header.Add(testCase.headerName, value)
			}

			_, responseError := New(testCase.options...).Parse(request)
			if responseError == nil {
				t.Fatalf("expected a refusal")
			}

			var got string
			for _, header := range responseError.Headers {
				if header != nil && header.Name == "WWW-Authenticate" {
					got = header.Value
				}
			}
			if got != testCase.wantChallenge {
				t.Errorf("challenge = %q, want %q", got, testCase.wantChallenge)
			}
		})
	}
}
