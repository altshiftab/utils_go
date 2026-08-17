package request_parser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/processor"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
)

func TestRequestParserWithProcessor_Parse(t *testing.T) {
	t.Parallel()

	stringParser := func(value string, responseError *response_error.ResponseError) RequestParser[string] {
		return New(func(*http.Request) (string, *response_error.ResponseError) {
			return value, responseError
		})
	}
	lengthProcessor := processor.New(func(_ context.Context, s string) (int, *response_error.ResponseError) {
		return len(s), nil
	})

	t.Run("success runs parser then processor", func(t *testing.T) {
		t.Parallel()
		parser := NewWithProcessor[string, int](stringParser("hello", nil), lengthProcessor)
		result, responseError := parser.Parse(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if result != 5 {
			t.Fatalf("got %d, want 5", result)
		}
	})

	t.Run("parser error propagates", func(t *testing.T) {
		t.Parallel()
		parseError := &response_error.ResponseError{ProblemDetail: problem_detail.New(http.StatusBadRequest)}
		parser := NewWithProcessor[string, int](stringParser("", parseError), lengthProcessor)
		_, responseError := parser.Parse(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
		if responseError != parseError {
			t.Fatalf("expected the parser error, got %#v", responseError)
		}
	})

	t.Run("processor error propagates", func(t *testing.T) {
		t.Parallel()
		processError := &response_error.ResponseError{ProblemDetail: problem_detail.New(http.StatusUnprocessableEntity)}
		failingProcessor := processor.New(func(_ context.Context, _ string) (int, *response_error.ResponseError) {
			return 0, processError
		})
		parser := NewWithProcessor[string, int](stringParser("hi", nil), failingProcessor)
		_, responseError := parser.Parse(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
		if responseError != processError {
			t.Fatalf("expected the processor error, got %#v", responseError)
		}
	})

	t.Run("nil request parser", func(t *testing.T) {
		t.Parallel()
		parser := &RequestParserWithProcessor[string, int]{Processor: lengthProcessor}
		_, responseError := parser.Parse(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})

	t.Run("nil processor", func(t *testing.T) {
		t.Parallel()
		parser := &RequestParserWithProcessor[string, int]{RequestParser: stringParser("x", nil)}
		_, responseError := parser.Parse(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})
}
