package mux

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	muxTypesStaticContent "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint/static_content"
)

func TestObtainRequestBody(t *testing.T) {
	t.Parallel()

	t.Run("nil reader is a server error", func(t *testing.T) {
		t.Parallel()
		_, responseError := ObtainRequestBody(t.Context(), 5, nil, 0)
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})

	t.Run("negative content length yields no body", func(t *testing.T) {
		t.Parallel()
		body, responseError := ObtainRequestBody(t.Context(), -1, io.NopCloser(strings.NewReader("x")), 0)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if body != nil {
			t.Fatalf("expected nil body, got %q", body)
		}
	})

	t.Run("over the limit is 413", func(t *testing.T) {
		t.Parallel()
		_, responseError := ObtainRequestBody(t.Context(), 10, io.NopCloser(strings.NewReader("0123456789")), 5)
		if responseError == nil || responseError.ProblemDetail == nil {
			t.Fatalf("expected a problem detail, got %#v", responseError)
		}
		if responseError.ProblemDetail.Status != http.StatusRequestEntityTooLarge {
			t.Fatalf("expected 413, got %d", responseError.ProblemDetail.Status)
		}
	})

	t.Run("reads the body", func(t *testing.T) {
		t.Parallel()
		body, responseError := ObtainRequestBody(t.Context(), 5, io.NopCloser(strings.NewReader("hello")), 0)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if string(body) != "hello" {
			t.Fatalf("body = %q, want %q", body, "hello")
		}
	})
}

func TestGetEndpoint(t *testing.T) {
	t.Parallel()

	getEndpoint := &endpointPkg.Endpoint{Path: "/x", Method: http.MethodGet}
	specMap := map[string]map[string]*endpointPkg.Endpoint{
		"/x": {http.MethodGet: getEndpoint},
	}

	t.Run("empty map is 404", func(t *testing.T) {
		t.Parallel()
		_, _, responseError := GetEndpoint(nil, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil))
		if responseError == nil || responseError.ProblemDetail == nil || responseError.ProblemDetail.Status != http.StatusNotFound {
			t.Fatalf("expected 404, got %#v", responseError)
		}
	})

	t.Run("nil request is a server error", func(t *testing.T) {
		t.Parallel()
		_, _, responseError := GetEndpoint(specMap, nil)
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})

	t.Run("nil url is a server error", func(t *testing.T) {
		t.Parallel()
		_, _, responseError := GetEndpoint(specMap, &http.Request{Method: http.MethodGet})
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})

	t.Run("unknown path is 404", func(t *testing.T) {
		t.Parallel()
		_, _, responseError := GetEndpoint(specMap, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/unknown", nil))
		if responseError == nil || responseError.ProblemDetail == nil || responseError.ProblemDetail.Status != http.StatusNotFound {
			t.Fatalf("expected 404, got %#v", responseError)
		}
	})

	t.Run("method match returns the endpoint", func(t *testing.T) {
		t.Parallel()
		endpoint, _, responseError := GetEndpoint(specMap, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil))
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if endpoint != getEndpoint {
			t.Fatalf("expected the GET endpoint, got %#v", endpoint)
		}
	})

	t.Run("HEAD is treated as GET", func(t *testing.T) {
		t.Parallel()
		endpoint, _, responseError := GetEndpoint(specMap, httptest.NewRequestWithContext(t.Context(), http.MethodHead, "/x", nil))
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if endpoint != getEndpoint {
			t.Fatalf("expected the GET endpoint for HEAD, got %#v", endpoint)
		}
	})

	t.Run("unknown method returns the method map without an error", func(t *testing.T) {
		t.Parallel()
		endpoint, methodMap, responseError := GetEndpoint(specMap, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/x", nil))
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if endpoint != nil {
			t.Fatalf("expected a nil endpoint, got %#v", endpoint)
		}
		if methodMap == nil {
			t.Fatal("expected the method map to be returned")
		}
	})
}

func TestObtainIsCached(t *testing.T) {
	t.Parallel()

	staticContent := &muxTypesStaticContent.StaticContent{
		StaticContentData: muxTypesStaticContent.StaticContentData{
			Etag:         `"v1"`,
			LastModified: "Mon, 01 Jan 2000 00:00:00 GMT",
		},
	}

	t.Run("nil static content is a server error", func(t *testing.T) {
		t.Parallel()
		_, responseError := ObtainIsCached(nil, http.Header{})
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})

	t.Run("nil header is a server error", func(t *testing.T) {
		t.Parallel()
		_, responseError := ObtainIsCached(staticContent, nil)
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})

	t.Run("matching etag is a hit", func(t *testing.T) {
		t.Parallel()
		cached, responseError := ObtainIsCached(staticContent, http.Header{"If-None-Match": {`"v1"`}})
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if !cached {
			t.Fatal("expected a cache hit")
		}
	})

	t.Run("no conditional headers is a miss", func(t *testing.T) {
		t.Parallel()
		cached, responseError := ObtainIsCached(staticContent, http.Header{})
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if cached {
			t.Fatal("expected a cache miss")
		}
	})

	t.Run("malformed if-modified-since is 400", func(t *testing.T) {
		t.Parallel()
		_, responseError := ObtainIsCached(staticContent, http.Header{"If-Modified-Since": {"not-a-date"}})
		if responseError == nil || responseError.ProblemDetail == nil {
			t.Fatalf("expected a problem detail, got %#v", responseError)
		}
		if responseError.ProblemDetail.Status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", responseError.ProblemDetail.Status)
		}
	})
}

func TestObtainStaticContentResponse(t *testing.T) {
	t.Parallel()

	staticContent := &muxTypesStaticContent.StaticContent{
		StaticContentData: muxTypesStaticContent.StaticContentData{Data: []byte("body")},
	}

	t.Run("nil static content is a server error", func(t *testing.T) {
		t.Parallel()
		_, responseError := ObtainStaticContentResponse(nil, false, http.Header{}, nil)
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})

	t.Run("nil header is a server error", func(t *testing.T) {
		t.Parallel()
		_, responseError := ObtainStaticContentResponse(staticContent, false, nil, nil)
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})

	t.Run("cached is 304", func(t *testing.T) {
		t.Parallel()
		response, responseError := ObtainStaticContentResponse(staticContent, true, http.Header{}, nil)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if response == nil || response.StatusCode != http.StatusNotModified {
			t.Fatalf("expected 304, got %#v", response)
		}
	})

	t.Run("identity response is 200 with body", func(t *testing.T) {
		t.Parallel()
		response, responseError := ObtainStaticContentResponse(staticContent, false, http.Header{}, nil)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if response == nil || response.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %#v", response)
		}
		if string(response.Body) != "body" {
			t.Fatalf("body = %q, want %q", response.Body, "body")
		}
	})
}
