package openapi

import (
	"net/http"
	"strings"
	"testing"

	muxResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	"github.com/altshiftab/utils_go/pkg/http/openapi/openapi_config"
)

// headerValue reads one header off the static content, so that a case can say what the served
// document announces about itself.
func headerValue(headers []*muxResponse.HeaderEntry, name string) string {
	for _, header := range headers {
		if header != nil && strings.EqualFold(header.Name, name) {
			return header.Value
		}
	}

	return ""
}

func TestNewEndpoint(t *testing.T) {
	t.Parallel()

	data := []byte("{\"openapi\": \"3.2.0\"}\n")

	testCases := []struct {
		name                 string
		options              []openapi_config.Option
		expectedPath         string
		expectedPublic       bool
		expectedCacheControl string
	}{
		{
			name:                 "gated by default",
			expectedPath:         openapi_config.DefaultPath,
			expectedPublic:       false,
			expectedCacheControl: "private, no-cache",
		},
		{
			name:                 "public when asked",
			options:              []openapi_config.Option{openapi_config.WithPublic(true)},
			expectedPath:         openapi_config.DefaultPath,
			expectedPublic:       true,
			expectedCacheControl: "public, max-age=300",
		},
		{
			name:                 "served where the caller says",
			options:              []openapi_config.Option{openapi_config.WithPath("/api/openapi.json")},
			expectedPath:         "/api/openapi.json",
			expectedPublic:       false,
			expectedCacheControl: "private, no-cache",
		},
		{
			name: "cached as the caller says",
			options: []openapi_config.Option{
				openapi_config.WithPublic(true),
				openapi_config.WithCacheControl("public, max-age=60"),
			},
			expectedPath:         openapi_config.DefaultPath,
			expectedPublic:       true,
			expectedCacheControl: "public, max-age=60",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			endpoint, err := NewEndpoint(data, testCase.options...)
			if err != nil {
				t.Fatalf("%s: %v", testCase.name, err)
			}

			if endpoint.Path != testCase.expectedPath {
				t.Errorf("%s: served at %q, expected %q", testCase.name, endpoint.Path, testCase.expectedPath)
			}
			if endpoint.Method != http.MethodGet {
				t.Errorf("%s: served as %s", testCase.name, endpoint.Method)
			}
			if endpoint.Public != testCase.expectedPublic {
				t.Errorf("%s: public is %v, expected %v", testCase.name, endpoint.Public, testCase.expectedPublic)
			}

			staticContent := endpoint.StaticContent
			if staticContent == nil {
				t.Fatalf("%s: the document is not served as static content", testCase.name)
			}

			if string(staticContent.Data) != string(data) {
				t.Errorf("%s: the served body is not the document", testCase.name)
			}
			if staticContent.Etag == "" {
				t.Errorf("%s: the served document carries no etag", testCase.name)
			}

			if contentType := headerValue(staticContent.Headers, "Content-Type"); contentType != openapi_config.ContentType {
				t.Errorf("%s: served as %q", testCase.name, contentType)
			}

			cacheControl := headerValue(staticContent.Headers, "Cache-Control")
			if cacheControl != testCase.expectedCacheControl {
				t.Errorf(
					"%s: cached as %q, expected %q",
					testCase.name,
					cacheControl,
					testCase.expectedCacheControl,
				)
			}
		})
	}
}

// TestNewEndpointRefusesNothing checks that an empty document is refused rather than served, since
// an endpoint answering with no body says the API has no operations.
func TestNewEndpointRefusesNothing(t *testing.T) {
	t.Parallel()

	if _, err := NewEndpoint(nil); err == nil {
		t.Error("an empty document was accepted")
	}

	if _, err := NewEndpoint([]byte("{}"), openapi_config.WithPath("")); err == nil {
		t.Error("a document with nowhere to be served was accepted")
	}
}
