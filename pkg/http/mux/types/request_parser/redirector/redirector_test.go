package redirector

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/redirector/redirector_config"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
)

func locationHeader(responseError *response_error.ResponseError) (string, bool) {
	if responseError == nil {
		return "", false
	}
	for _, header := range responseError.Headers {
		if header != nil && header.Name == "Location" {
			return header.Value, true
		}
	}
	return "", false
}

func failingParser(status int) request_parser.RequestParser[string] {
	return request_parser.New(func(*http.Request) (string, *response_error.ResponseError) {
		return "", &response_error.ResponseError{ProblemDetail: problem_detail.New(status)}
	})
}

func newNavigationRequest(t *testing.T, navigate bool) *http.Request {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://app.example.com/protected", nil)
	if navigate {
		request.Header.Set("Sec-Fetch-Mode", "navigate")
	}
	return request
}

func TestParse_PassThrough(t *testing.T) {
	t.Parallel()

	redirectUrl, err := url.Parse("https://example.com/login")
	if err != nil {
		t.Fatalf("url parse: %v", err)
	}

	succeeding := request_parser.New(func(*http.Request) (string, *response_error.ResponseError) {
		return "ok", nil
	})

	parser, err := New(succeeding, redirectUrl)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	value, responseError := parser.Parse(newNavigationRequest(t, true))
	if responseError != nil {
		t.Fatalf("unexpected error: %#v", responseError)
	}
	if value != "ok" {
		t.Fatalf("got %q, want %q", value, "ok")
	}
}

func TestParse_RedirectGating(t *testing.T) {
	t.Parallel()

	redirectUrl, err := url.Parse("https://example.com/login")
	if err != nil {
		t.Fatalf("url parse: %v", err)
	}

	testCases := []struct {
		name         string
		status       int
		navigate     bool
		wantRedirect bool
	}{
		{name: "401 navigation redirects", status: http.StatusUnauthorized, navigate: true, wantRedirect: true},
		{name: "401 non-navigation does not redirect", status: http.StatusUnauthorized, navigate: false},
		{name: "403 navigation does not redirect", status: http.StatusForbidden, navigate: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			parser, err := New(failingParser(testCase.status), redirectUrl)
			if err != nil {
				t.Fatalf("new: %v", err)
			}

			_, responseError := parser.Parse(newNavigationRequest(t, testCase.navigate))

			location, ok := locationHeader(responseError)
			if ok != testCase.wantRedirect {
				t.Fatalf("redirect present = %v, want %v (%#v)", ok, testCase.wantRedirect, responseError)
			}
			if testCase.wantRedirect && !strings.HasPrefix(location, "https://example.com/login?redirect=") {
				t.Fatalf("unexpected Location: %q", location)
			}
		})
	}
}

func TestParse_RequireProto(t *testing.T) {
	t.Parallel()

	redirectUrl, err := url.Parse("https://example.com/login")
	if err != nil {
		t.Fatalf("url parse: %v", err)
	}

	parser, err := New(
		failingParser(http.StatusUnauthorized),
		redirectUrl,
		redirector_config.WithRequireProto(true),
	)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	_, responseError := parser.Parse(newNavigationRequest(t, true))
	if responseError == nil || responseError.ServerError == nil {
		t.Fatalf("expected a server error for missing X-Forwarded-Proto, got %#v", responseError)
	}
}

func TestParse_SchemeInRedirectParameter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		headers  map[string]string
		expected string
	}{
		{
			name:     "forwarded is preferred",
			headers:  map[string]string{"Forwarded": "proto=https", "X-Forwarded-Proto": "http"},
			expected: "https%3A%2F%2Fapp.example.com%2Fprotected",
		},
		{
			name:     "x-forwarded-proto is used when forwarded says nothing",
			headers:  map[string]string{"X-Forwarded-Proto": "https"},
			expected: "https%3A%2F%2Fapp.example.com%2Fprotected",
		},
		{
			name:     "a rejected proto falls back to the connection",
			headers:  map[string]string{"X-Forwarded-Proto": "javascript"},
			expected: "http%3A%2F%2Fapp.example.com%2Fprotected",
		},
	}

	redirectUrl, err := url.Parse("https://example.com/login")
	if err != nil {
		t.Fatalf("url parse: %v", err)
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			parser, err := New(failingParser(http.StatusUnauthorized), redirectUrl)
			if err != nil {
				t.Fatalf("new: %v", err)
			}

			request := newNavigationRequest(t, true)
			for name, value := range testCase.headers {
				request.Header.Set(name, value)
			}

			_, responseError := parser.Parse(request)
			location, ok := locationHeader(responseError)
			if !ok {
				t.Fatalf("expected a Location header, got %#v", responseError)
			}
			if !strings.Contains(location, testCase.expected) {
				t.Fatalf("got %q, want it to contain %q", location, testCase.expected)
			}
		})
	}
}

func TestParse_RequireProtoAcceptsForwarded(t *testing.T) {
	t.Parallel()

	redirectUrl, err := url.Parse("https://example.com/login")
	if err != nil {
		t.Fatalf("url parse: %v", err)
	}

	parser, err := New(
		failingParser(http.StatusUnauthorized),
		redirectUrl,
		redirector_config.WithRequireProto(true),
	)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	request := newNavigationRequest(t, true)
	request.Header.Set("Forwarded", "for=192.0.2.1;proto=https")

	_, responseError := parser.Parse(request)
	if responseError == nil || responseError.ServerError != nil {
		t.Fatalf("Forwarded should satisfy RequireProto, got %#v", responseError)
	}
	if _, ok := locationHeader(responseError); !ok {
		t.Fatalf("expected a Location header, got %#v", responseError)
	}
}
