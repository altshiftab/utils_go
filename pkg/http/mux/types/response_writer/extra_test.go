package response_writer

import (
	"net/http"
	"net/http/httptest"
	"testing"

	muxTypesResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
)

func TestWriteResponse_HeadRequestOmitsBody(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	writer := &ResponseWriter{ResponseWriter: recorder, IsHeadRequest: true}

	if err := writer.WriteResponse(t.Context(), &muxTypesResponse.Response{StatusCode: http.StatusOK, Body: []byte("hello")}, nil); err != nil {
		t.Fatalf("write response: %v", err)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("expected no body for a HEAD request, got %q", recorder.Body.String())
	}
}

func TestWriteResponse_MalformedContentType(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	writer := &ResponseWriter{ResponseWriter: recorder}

	err := writer.WriteResponse(t.Context(), &muxTypesResponse.Response{
		Body:    []byte("x"),
		Headers: []*muxTypesResponse.HeaderEntry{{Name: "Content-Type", Value: "not a valid content-type at all"}},
	}, nil)
	if err == nil {
		t.Fatal("expected an error for a malformed Content-Type")
	}
}

func TestWriteResponse_VaryHeader(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	writer := &ResponseWriter{ResponseWriter: recorder}

	// A Vary header is emitted only when the response is cacheable (Cache-Control is
	// overridden away from the no-store default).
	response := &muxTypesResponse.Response{
		Body: []byte("x"),
		Headers: []*muxTypesResponse.HeaderEntry{
			{Name: "Cache-Control", Value: "max-age=60", Overwrite: true},
			{Name: "Vary", Value: "Accept-Encoding"},
		},
	}
	if err := writer.WriteResponse(t.Context(), response, nil); err != nil {
		t.Fatalf("write response: %v", err)
	}
	if got := recorder.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Errorf("Vary = %q, want Accept-Encoding", got)
	}
}
