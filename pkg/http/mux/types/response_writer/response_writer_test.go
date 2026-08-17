package response_writer

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	muxTypesResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	acceptEncodingParsing "github.com/altshiftab/utils_go/pkg/http/types/accept_encoding"
)

var errStream = errors.New("stream failure")

func writeAndRecord(t *testing.T, response *muxTypesResponse.Response) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	writer := &ResponseWriter{ResponseWriter: recorder}
	if err := writer.WriteResponse(t.Context(), response, nil); err != nil {
		t.Fatalf("write response: %v", err)
	}
	return recorder
}

func TestWriteResponse_DefaultHeaders(t *testing.T) {
	t.Parallel()

	recorder := writeAndRecord(t, &muxTypesResponse.Response{})

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	for name, want := range map[string]string{
		"Cache-Control":                "no-store",
		"X-Content-Type-Options":       "nosniff",
		"Cross-Origin-Resource-Policy": "same-origin",
	} {
		if got := recorder.Header().Get(name); got != want {
			t.Errorf("header %q = %q, want %q", name, got, want)
		}
	}
}

func TestWriteResponse_CustomHeaderAndStatus(t *testing.T) {
	t.Parallel()

	recorder := writeAndRecord(t, &muxTypesResponse.Response{
		StatusCode: http.StatusCreated,
		Body:       []byte("hello"),
		Headers:    []*muxTypesResponse.HeaderEntry{{Name: "X-Custom", Value: "value"}},
	})

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	if recorder.Body.String() != "hello" {
		t.Fatalf("body = %q, want %q", recorder.Body.String(), "hello")
	}
	if got := recorder.Header().Get("X-Custom"); got != "value" {
		t.Fatalf("X-Custom = %q, want %q", got, "value")
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("default Cache-Control = %q, want no-store", got)
	}
}

func TestWriteResponse_Overwrite(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		overwrite bool
		want      string
	}{
		{name: "overwrite replaces default", overwrite: true, want: "max-age=60"},
		{name: "no overwrite keeps default", overwrite: false, want: "no-store"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			recorder := writeAndRecord(t, &muxTypesResponse.Response{
				Headers: []*muxTypesResponse.HeaderEntry{
					{Name: "Cache-Control", Value: "max-age=60", Overwrite: testCase.overwrite},
				},
			})
			if got := recorder.Header().Get("Cache-Control"); got != testCase.want {
				t.Fatalf("Cache-Control = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestWriteResponse_DocumentHeaders(t *testing.T) {
	t.Parallel()

	recorder := writeAndRecord(t, &muxTypesResponse.Response{
		Body:    []byte("<html></html>"),
		Headers: []*muxTypesResponse.HeaderEntry{{Name: "Content-Type", Value: "text/html"}},
	})

	if got := recorder.Header().Get("Content-Type"); got != "text/html" {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}
	if recorder.Header().Get("Content-Security-Policy") == "" {
		t.Error("expected a Content-Security-Policy header for a document response")
	}
	if got := recorder.Header().Get("Cross-Origin-Opener-Policy"); got != "same-origin" {
		t.Errorf("Cross-Origin-Opener-Policy = %q, want same-origin", got)
	}
}

func TestWriteResponse_Compression(t *testing.T) {
	t.Parallel()

	acceptEncoding, err := acceptEncodingParsing.Parse([]byte("gzip"))
	if err != nil {
		t.Fatalf("parse accept encoding: %v", err)
	}

	// A large, compressible, non-sensitive body with a no-store default triggers gzip.
	body := bytes.Repeat([]byte("a"), 2000)

	t.Run("compresses a large body", func(t *testing.T) {
		t.Parallel()
		recorder := httptest.NewRecorder()
		writer := &ResponseWriter{ResponseWriter: recorder}
		if err := writer.WriteResponse(t.Context(), &muxTypesResponse.Response{Body: body}, acceptEncoding); err != nil {
			t.Fatalf("write response: %v", err)
		}
		if got := recorder.Header().Get("Content-Encoding"); got != "gzip" {
			t.Fatalf("Content-Encoding = %q, want gzip", got)
		}
		if recorder.Body.Len() >= len(body) {
			t.Errorf("expected the gzipped body (%d) to be smaller than %d", recorder.Body.Len(), len(body))
		}
	})

	t.Run("sensitive body is not compressed", func(t *testing.T) {
		t.Parallel()
		recorder := httptest.NewRecorder()
		writer := &ResponseWriter{ResponseWriter: recorder}
		response := &muxTypesResponse.Response{Body: body, SensitiveBody: true}
		if err := writer.WriteResponse(t.Context(), response, acceptEncoding); err != nil {
			t.Fatalf("write response: %v", err)
		}
		if got := recorder.Header().Get("Content-Encoding"); got != "" {
			t.Fatalf("expected no Content-Encoding for a sensitive body, got %q", got)
		}
	})
}

func TestWriteResponse_Streaming(t *testing.T) {
	t.Parallel()

	t.Run("streams chunks", func(t *testing.T) {
		t.Parallel()
		recorder := httptest.NewRecorder()
		writer := &ResponseWriter{ResponseWriter: recorder}
		streamer := func(yield func([]byte, error) bool) {
			if !yield([]byte("chunk-1;"), nil) {
				return
			}
			yield([]byte("chunk-2"), nil)
		}
		if err := writer.WriteResponse(t.Context(), &muxTypesResponse.Response{BodyStreamer: streamer}, nil); err != nil {
			t.Fatalf("write response: %v", err)
		}
		if got := recorder.Body.String(); got != "chunk-1;chunk-2" {
			t.Fatalf("body = %q, want %q", got, "chunk-1;chunk-2")
		}
		if got := recorder.Header().Get("Transfer-Encoding"); got != "chunked" {
			t.Errorf("Transfer-Encoding = %q, want chunked", got)
		}
	})

	t.Run("stream error is returned", func(t *testing.T) {
		t.Parallel()
		recorder := httptest.NewRecorder()
		writer := &ResponseWriter{ResponseWriter: recorder}
		streamer := func(yield func([]byte, error) bool) {
			yield(nil, errStream)
		}
		if err := writer.WriteResponse(t.Context(), &muxTypesResponse.Response{BodyStreamer: streamer}, nil); err == nil {
			t.Fatal("expected an error from the stream")
		}
	})
}

// TestWriteResponse_DocumentHeadersReplace verifies that what is said about a document replaces
// what is said about a response in general, rather than being said alongside it. A browser enforces
// every content security policy it is sent, so a document carrying two would be held to what the
// two permit between them.
func TestWriteResponse_DocumentHeadersReplace(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		contentType    string
		expectedPolicy string
	}{
		{
			name:           "a response a browser does not render",
			contentType:    "application/json",
			expectedPolicy: "default-src 'none'",
		},
		{
			name:           "a document",
			contentType:    "text/html",
			expectedPolicy: "default-src 'self'",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			writer := &ResponseWriter{
				ResponseWriter:         recorder,
				DefaultHeaders:         map[string]string{contentSecurityPolicyHeaderName: "default-src 'none'"},
				DefaultDocumentHeaders: map[string]string{contentSecurityPolicyHeaderName: "default-src 'self'"},
			}

			response := &muxTypesResponse.Response{
				Body:    []byte("body"),
				Headers: []*muxTypesResponse.HeaderEntry{{Name: "Content-Type", Value: testCase.contentType}},
			}

			if err := writer.WriteResponse(t.Context(), response, nil); err != nil {
				t.Fatalf("write response: %v", err)
			}

			policies := recorder.Header().Values(contentSecurityPolicyHeaderName)
			if len(policies) != 1 {
				t.Fatalf("content security policy: got %d of them, want one: %v", len(policies), policies)
			}
			if policies[0] != testCase.expectedPolicy {
				t.Errorf("content security policy: got %q, want %q", policies[0], testCase.expectedPolicy)
			}
		})
	}
}
