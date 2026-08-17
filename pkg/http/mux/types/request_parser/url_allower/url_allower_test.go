package url_allower

import (
	"net/http"
	"testing"

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
		{name: "allowed registered domain", url: "http://example.com/", allowLocalhost: true, allowed: true},
		{name: "uppercase allowed registered domain", url: "http://EXAMPLE.COM/", allowLocalhost: true, allowed: true},
		{name: "disallowed registered domain", url: "http://example.org/", allowLocalhost: true, allowed: false},
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
