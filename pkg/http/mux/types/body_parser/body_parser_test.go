package body_parser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/processor"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
)

func TestBodyParserWithProcessor_Parse(t *testing.T) {
	t.Parallel()

	stringParser := func(value string, responseError *response_error.ResponseError) BodyParser[string] {
		return New(func(*http.Request, []byte) (string, *response_error.ResponseError) {
			return value, responseError
		})
	}
	lengthProcessor := processor.New(func(_ context.Context, s string) (int, *response_error.ResponseError) {
		return len(s), nil
	})

	newRequest := func(t *testing.T) *http.Request {
		t.Helper()
		return httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
	}

	t.Run("success runs parser then processor", func(t *testing.T) {
		t.Parallel()
		parser := NewWithProcessor[string, int](stringParser("hello", nil), lengthProcessor)
		result, responseError := parser.Parse(newRequest(t), []byte("ignored"))
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
		_, responseError := parser.Parse(newRequest(t), nil)
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
		_, responseError := parser.Parse(newRequest(t), nil)
		if responseError != processError {
			t.Fatalf("expected the processor error, got %#v", responseError)
		}
	})

	t.Run("nil body parser", func(t *testing.T) {
		t.Parallel()
		parser := &BodyParserWithProcessor[string, int]{Processor: lengthProcessor}
		_, responseError := parser.Parse(newRequest(t), nil)
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})

	t.Run("nil processor", func(t *testing.T) {
		t.Parallel()
		parser := &BodyParserWithProcessor[string, int]{BodyParser: stringParser("x", nil)}
		_, responseError := parser.Parse(newRequest(t), nil)
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})
}
