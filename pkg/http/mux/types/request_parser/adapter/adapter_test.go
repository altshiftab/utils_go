package adapter

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
)

func TestAdapter_Parse(t *testing.T) {
	t.Parallel()

	inner := request_parser.New(func(*http.Request) (string, *response_error.ResponseError) {
		return "value", nil
	})
	adapter := New[string](inner)

	result, responseError := adapter.Parse(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	if responseError != nil {
		t.Fatalf("unexpected error: %#v", responseError)
	}

	parsed, ok := result.(string)
	if !ok || parsed != "value" {
		t.Fatalf("got %v (%T), want %q", result, result, "value")
	}
}
