package validatable_processor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

type testValidatable struct {
	err error
}

func (v *testValidatable) Validate() error { return v.err }

var errNotValidation = errors.New("boom")

func TestNew(t *testing.T) {
	t.Parallel()

	proc := New[*testValidatable]()

	t.Run("valid input passes through", func(t *testing.T) {
		t.Parallel()
		input := &testValidatable{}
		result, responseError := proc.Process(t.Context(), input)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if result != input {
			t.Fatal("expected the input to pass through")
		}
	})

	t.Run("validation error becomes 400", func(t *testing.T) {
		t.Parallel()
		input := &testValidatable{err: fmt.Errorf("%w: bad", altshiftErrors.ErrValidationError)}
		_, responseError := proc.Process(t.Context(), input)
		if responseError == nil || responseError.ProblemDetail == nil {
			t.Fatalf("expected a problem detail, got %#v", responseError)
		}
		if responseError.ProblemDetail.Status != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, responseError.ProblemDetail.Status)
		}
	})

	t.Run("non-validation error is ignored", func(t *testing.T) {
		t.Parallel()
		input := &testValidatable{err: errNotValidation}
		result, responseError := proc.Process(t.Context(), input)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if result != input {
			t.Fatal("expected the input to pass through")
		}
	})

	t.Run("nil input", func(t *testing.T) {
		t.Parallel()
		result, responseError := proc.Process(t.Context(), nil)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if result != nil {
			t.Fatalf("expected a nil result, got %#v", result)
		}
	})

	t.Run("cancelled context is a server error", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, responseError := proc.Process(ctx, &testValidatable{})
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})
}
