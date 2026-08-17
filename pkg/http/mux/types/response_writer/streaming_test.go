package response_writer

import (
	"bytes"
	cryptorand "crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	muxTypesResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	acceptEncodingParsing "github.com/altshiftab/utils_go/pkg/http/types/accept_encoding"
)

// headerOnlyWriter implements http.ResponseWriter but not http.Flusher.
type headerOnlyWriter struct {
	header http.Header
	code   int
}

func (w *headerOnlyWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}

func (w *headerOnlyWriter) Write(data []byte) (int, error) { return len(data), nil }

func (w *headerOnlyWriter) WriteHeader(code int) { w.code = code }

func TestWriteResponse_StreamingTransferEncodingAlreadySet(t *testing.T) {
	t.Parallel()

	writer := &ResponseWriter{ResponseWriter: httptest.NewRecorder()}
	response := &muxTypesResponse.Response{
		BodyStreamer: func(yield func([]byte, error) bool) { yield([]byte("x"), nil) },
		Headers:      []*muxTypesResponse.HeaderEntry{{Name: "Transfer-Encoding", Value: "chunked"}},
	}
	if err := writer.WriteResponse(t.Context(), response, nil); err == nil {
		t.Fatal("expected an error when Transfer-Encoding is already set")
	}
}

func TestWriteResponse_StreamingWithoutFlusher(t *testing.T) {
	t.Parallel()

	writer := &ResponseWriter{ResponseWriter: &headerOnlyWriter{}}
	response := &muxTypesResponse.Response{
		BodyStreamer: func(yield func([]byte, error) bool) { yield([]byte("x"), nil) },
	}
	if err := writer.WriteResponse(t.Context(), response, nil); err == nil {
		t.Fatal("expected an error when the writer is not a flusher")
	}
}

func TestWriteResponse_IncompressibleBodyIsNotCompressed(t *testing.T) {
	t.Parallel()

	acceptEncoding, err := acceptEncodingParsing.Parse([]byte("gzip"))
	if err != nil {
		t.Fatalf("parse accept encoding: %v", err)
	}

	// Random bytes do not compress, so gzip is not applied.
	body := make([]byte, 2000)
	if _, err := cryptorand.Read(body); err != nil {
		t.Fatalf("rand read: %v", err)
	}

	recorder := httptest.NewRecorder()
	writer := &ResponseWriter{ResponseWriter: recorder}
	if err := writer.WriteResponse(t.Context(), &muxTypesResponse.Response{Body: body}, acceptEncoding); err != nil {
		t.Fatalf("write response: %v", err)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("expected no Content-Encoding for incompressible data, got %q", got)
	}
}

func TestWriteResponse_PreEncodedBodySkipsCompression(t *testing.T) {
	t.Parallel()

	acceptEncoding, err := acceptEncodingParsing.Parse([]byte("gzip"))
	if err != nil {
		t.Fatalf("parse accept encoding: %v", err)
	}

	recorder := httptest.NewRecorder()
	writer := &ResponseWriter{ResponseWriter: recorder}
	response := &muxTypesResponse.Response{
		Body:    bytes.Repeat([]byte("a"), 2000),
		Headers: []*muxTypesResponse.HeaderEntry{{Name: "Content-Encoding", Value: "br"}},
	}
	if err := writer.WriteResponse(t.Context(), response, acceptEncoding); err != nil {
		t.Fatalf("write response: %v", err)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != "br" {
		t.Errorf("Content-Encoding = %q, want br (compression should be skipped)", got)
	}
}
