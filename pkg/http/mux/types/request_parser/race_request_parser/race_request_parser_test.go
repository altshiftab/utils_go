package race_request_parser

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
)

func succeedingParser(value string) request_parser.RequestParser[*string] {
	v := value
	return request_parser.New(func(*http.Request) (*string, *response_error.ResponseError) {
		return &v, nil
	})
}

func failingParser(status int) request_parser.RequestParser[*string] {
	return request_parser.New(func(*http.Request) (*string, *response_error.ResponseError) {
		return nil, &response_error.ResponseError{ProblemDetail: problem_detail.New(status)}
	})
}

func TestParse_NilRequest(t *testing.T) {
	t.Parallel()

	parser := New([]request_parser.RequestParser[*string]{succeedingParser("a")})
	_, responseError := parser.Parse(nil)
	if responseError == nil || responseError.ServerError == nil {
		t.Fatalf("expected a server error, got %#v", responseError)
	}
}

func TestParse_FirstSuccessWins(t *testing.T) {
	t.Parallel()

	parser := New([]request_parser.RequestParser[*string]{
		failingParser(http.StatusUnauthorized),
		succeedingParser("token"),
	})

	result, responseError := parser.Parse(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	if responseError != nil {
		t.Fatalf("unexpected error: %#v", responseError)
	}
	if result == nil || *result != "token" {
		t.Fatalf("got %v, want %q", result, "token")
	}
}

func TestParse_NilParserSkipped(t *testing.T) {
	t.Parallel()

	parser := New([]request_parser.RequestParser[*string]{nil, succeedingParser("value")})

	result, responseError := parser.Parse(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	if responseError != nil {
		t.Fatalf("unexpected error: %#v", responseError)
	}
	if result == nil || *result != "value" {
		t.Fatalf("got %v, want %q", result, "value")
	}
}

func TestParse_AllFailingAggregates(t *testing.T) {
	t.Parallel()

	parser := New([]request_parser.RequestParser[*string]{
		failingParser(http.StatusUnauthorized),
		failingParser(http.StatusForbidden),
	})

	result, responseError := parser.Parse(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}
	if responseError == nil || responseError.ProblemDetail == nil {
		t.Fatalf("expected a problem detail, got %#v", responseError)
	}
	if responseError.ProblemDetail.Status != http.StatusUnauthorized {
		t.Fatalf("expected aggregated status %d, got %d", http.StatusUnauthorized, responseError.ProblemDetail.Status)
	}
}
