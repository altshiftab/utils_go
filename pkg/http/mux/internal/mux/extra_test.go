package mux

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint/static_content"
	muxTypesRateLimiting "github.com/altshiftab/utils_go/pkg/http/mux/types/rate_limiting"
	acceptEncodingParsing "github.com/altshiftab/utils_go/pkg/http/types/accept_encoding"
)

var errKey = errors.New("key failure")

func TestObtainStaticContentResponse_ContentEncoding(t *testing.T) {
	t.Parallel()

	acceptEncoding, err := acceptEncodingParsing.Parse([]byte("gzip"))
	if err != nil {
		t.Fatalf("parse accept encoding: %v", err)
	}

	staticContent := &static_content.StaticContent{
		StaticContentData: static_content.StaticContentData{Data: []byte("identity")},
		ContentEncodingToData: map[string]*static_content.StaticContentData{
			"gzip": {Data: []byte("gzipped")},
		},
	}

	response, responseError := ObtainStaticContentResponse(staticContent, false, http.Header{}, acceptEncoding)
	if responseError != nil {
		t.Fatalf("unexpected error: %#v", responseError)
	}
	if response == nil {
		t.Fatal("expected a response")
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	if string(response.Body) != "gzipped" {
		t.Fatalf("body = %q, want gzipped", response.Body)
	}

	var contentEncoding string
	for _, header := range response.Headers {
		if header != nil && header.Name == "Content-Encoding" {
			contentEncoding = header.Value
		}
	}
	if contentEncoding != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", contentEncoding)
	}
}

func TestObtainStaticContentResponse_ContentEncodingDataMissing(t *testing.T) {
	t.Parallel()

	acceptEncoding, err := acceptEncodingParsing.Parse([]byte("gzip"))
	if err != nil {
		t.Fatalf("parse accept encoding: %v", err)
	}

	// A supported encoding whose data entry is nil is a server error.
	staticContent := &static_content.StaticContent{
		StaticContentData:     static_content.StaticContentData{Data: []byte("identity")},
		ContentEncodingToData: map[string]*static_content.StaticContentData{"gzip": nil},
	}

	_, responseError := ObtainStaticContentResponse(staticContent, false, http.Header{}, acceptEncoding)
	if responseError == nil || responseError.ServerError == nil {
		t.Fatalf("expected a server error, got %#v", responseError)
	}
}

func TestObtainRequestBody_MaxBytesExceeded(t *testing.T) {
	t.Parallel()

	body := io.NopCloser(strings.NewReader("0123456789"))
	limited := http.MaxBytesReader(httptest.NewRecorder(), body, 5)

	_, responseError := ObtainRequestBody(t.Context(), 10, limited, 0)
	if responseError == nil || responseError.ProblemDetail == nil {
		t.Fatalf("expected a problem detail, got %#v", responseError)
	}
	if responseError.ProblemDetail.Status != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", responseError.ProblemDetail.Status)
	}
}

func TestValidateContentType_Malformed(t *testing.T) {
	t.Parallel()

	header := http.Header{"Content-Type": {"this is not a valid content type"}}
	responseError := ValidateContentType("application/json", header)
	if responseError == nil {
		t.Fatal("expected an error for a malformed Content-Type")
	}
}

func TestHandleRateLimiting_KeyError(t *testing.T) {
	t.Parallel()

	config := &muxTypesRateLimiting.RateLimitingConfiguration{
		NumRequests:          1,
		NumSecondsExpiration: 5,
		GetKey: func(*http.Request) (string, error) {
			return "", errKey
		},
	}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)

	responseError := HandleRateLimiting(config, request)
	if responseError == nil || responseError.ServerError == nil {
		t.Fatalf("expected a server error, got %#v", responseError)
	}
}
