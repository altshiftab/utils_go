package url_allower

import (
	"errors"
	"net/http"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/url_allower/url_allower_config"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
)

type stringURL string

func (s stringURL) URL() string { return string(s) }

func TestParse_Localhost(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		url            string
		allowLocalhost bool
		allowed        bool
	}{
		{name: "localhost", url: "http://localhost/path", allowLocalhost: true, allowed: true},
		{name: "localhost with port", url: "http://localhost:8080/", allowLocalhost: true, allowed: true},
		{name: "localhost subdomain", url: "http://app.localhost/", allowLocalhost: true, allowed: true},
		{name: "nested localhost subdomain", url: "http://a.b.localhost/", allowLocalhost: true, allowed: true},
		{name: "localhost subdomain not allowed", url: "http://app.localhost/", allowLocalhost: false, allowed: false},
		{name: "uppercase localhost", url: "http://LOCALHOST/", allowLocalhost: true, allowed: true},
		{name: "mixed-case localhost subdomain", url: "http://App.LocalHost/", allowLocalhost: true, allowed: true},
		{name: "allowed registered domain", url: "https://example.com/", allowLocalhost: true, allowed: true},
		{name: "uppercase allowed registered domain", url: "https://EXAMPLE.COM/", allowLocalhost: true, allowed: true},
		{name: "disallowed registered domain", url: "https://example.org/", allowLocalhost: true, allowed: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			parser := New[stringURL](
				request_parser.New(func(*http.Request) (stringURL, *response_error.ResponseError) {
					return stringURL(testCase.url), nil
				}),
				url_allower_config.WithAllowLocalhost(testCase.allowLocalhost),
				url_allower_config.WithAllowedRegisteredDomains([]string{"example.com"}),
			)

			_, responseError := parser.Parse(&http.Request{})
			if testCase.allowed && responseError != nil {
				t.Fatalf("expected url to be allowed, got error: %v", responseError)
			}
			if !testCase.allowed && responseError == nil {
				t.Fatal("expected url to be rejected, got no error")
			}
		})
	}
}

// The domain checks read only a URL's hostname, which a URL of any scheme can
// carry, so what is tested is that only a destination a browser loads as a page
// gets through to them.
func TestParse_Scheme(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		url            string
		allowLocalhost bool
		withoutDomains bool
		allowed        bool
	}{
		{name: "https on an allowed domain", url: "https://a.example.com/p", allowed: true},
		{name: "https written in capitals", url: "HTTPS://a.example.com/p", allowed: true},
		{name: "https on localhost in a local run", url: "https://localhost:8443/", allowLocalhost: true, allowed: true},
		{name: "http on localhost in a local run", url: "http://localhost:8080/", allowLocalhost: true, allowed: true},
		{name: "http on a localhost subdomain in a local run", url: "http://app.localhost/", allowLocalhost: true, allowed: true},
		{name: "http on an allowed domain", url: "http://a.example.com/"},
		{name: "http on an allowed domain in a local run", url: "http://a.example.com/", allowLocalhost: true},
		{name: "javascript with an allowed host", url: "javascript://a.example.com/%0aalert(1)"},
		{name: "javascript with localhost in a local run", url: "javascript://localhost/%0aalert(1)", allowLocalhost: true},
		{name: "an intent with an allowed host", url: "intent://a.example.com/#Intent;scheme=https;end"},
		{name: "an app scheme with an allowed host", url: "someapp://a.example.com/p"},
		{name: "ftp with an allowed host", url: "ftp://a.example.com/"},
		{name: "relative to the scheme", url: "//a.example.com/x"},
		{name: "javascript where no domain is configured", url: "javascript://a.example.org/%0aalert(1)", withoutDomains: true},
		{name: "https where no domain is configured", url: "https://a.example.org/", withoutDomains: true, allowed: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			options := []url_allower_config.Option{url_allower_config.WithAllowLocalhost(testCase.allowLocalhost)}
			if !testCase.withoutDomains {
				options = append(options, url_allower_config.WithAllowedRegisteredDomains([]string{"example.com"}))
			}

			parser := New[stringURL](urlParser(testCase.url), options...)

			parsedUrl, responseError := parser.Parse(&http.Request{})
			if testCase.allowed {
				if responseError != nil {
					t.Fatalf("expected %q to be allowed, got %#v", testCase.url, responseError)
				}
				if parsedUrl == nil {
					t.Fatalf("expected a parsed url for %q", testCase.url)
				}
				return
			}

			if responseError == nil {
				t.Fatalf("expected %q to be refused, got %v", testCase.url, parsedUrl)
			}
			if responseError.ProblemDetail == nil || responseError.ProblemDetail.Status != http.StatusBadRequest {
				t.Fatalf("expected a 400 for %q, got %#v", testCase.url, responseError)
			}
			if !errors.Is(responseError.ClientError, altshiftErrors.ErrValidationError) {
				t.Fatalf("expected a validation error for %q, got %v", testCase.url, responseError.ClientError)
			}
		})
	}
}
