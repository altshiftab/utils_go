package choice_request_parser_config

import (
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
)

func TestNew_Default(t *testing.T) {
	t.Parallel()

	parser := New().ResponseErrorParser
	if parser == nil {
		t.Fatal("expected a default ResponseErrorParser")
	}

	// The default returns the first non-nil error.
	first := &response_error.ResponseError{ProblemDetail: problem_detail.New(http.StatusUnauthorized)}
	if got := parser([]*response_error.ResponseError{nil, first}); got != first {
		t.Fatalf("expected the first non-nil error, got %#v", got)
	}

	// With no errors, it falls back to a 400.
	got := parser([]*response_error.ResponseError{nil, nil})
	if got == nil || got.ProblemDetail == nil || got.ProblemDetail.Status != http.StatusBadRequest {
		t.Fatalf("expected a 400 fallback, got %#v", got)
	}
}

func TestNew_Override(t *testing.T) {
	t.Parallel()

	marker := &response_error.ResponseError{ProblemDetail: problem_detail.New(http.StatusTeapot)}
	parser := New(WithResponseErrorParser(func([]*response_error.ResponseError) *response_error.ResponseError {
		return marker
	})).ResponseErrorParser

	if got := parser(nil); got != marker {
		t.Fatalf("expected the overridden parser to be used, got %#v", got)
	}
}
