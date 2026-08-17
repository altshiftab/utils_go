package processor

import (
	"context"
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
)

func TestNew(t *testing.T) {
	t.Parallel()

	wantError := &response_error.ResponseError{ProblemDetail: problem_detail.New(http.StatusBadRequest)}

	proc := New(func(_ context.Context, n int) (string, *response_error.ResponseError) {
		if n < 0 {
			return "", wantError
		}
		return "positive", nil
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		result, responseError := proc.Process(t.Context(), 1)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if result != "positive" {
			t.Fatalf("got %q, want positive", result)
		}
	})

	t.Run("error propagates", func(t *testing.T) {
		t.Parallel()
		_, responseError := proc.Process(t.Context(), -1)
		if responseError != wantError {
			t.Fatalf("expected the error, got %#v", responseError)
		}
	})
}
