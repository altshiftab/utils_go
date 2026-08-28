package race_request_parser

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/race_request_parser/race_request_parser_config"
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

// admittingParser admits every request, as an identity of its own.
type admittingParser struct {
	identity string
}

func (parser *admittingParser) Parse(*http.Request) (any, *response_error.ResponseError) {
	return parser.identity, nil
}

// refusingParser admits nothing.
type refusingParser struct{}

func (*refusingParser) Parse(*http.Request) (any, *response_error.ResponseError) {
	return nil, &response_error.ResponseError{ClientError: errors.New("no")} //nolint:err113 // a stub's refusal.
}

// TestExclusiveRefusesTwoCredentials holds what the option is for. A request carrying two kinds of
// credential has two answers to "as whom", and picking one is guessing at what the sender meant: an
// audit trail then records an identity nobody chose.
func TestExclusiveRefusesTwoCredentials(t *testing.T) {
	t.Parallel()

	parser := New(
		[]request_parser.RequestParser[any]{
			&admittingParser{identity: "session"},
			&admittingParser{identity: "service account"},
		},
		race_request_parser_config.WithExclusive(),
	)

	result, responseError := parser.Parse(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	if responseError == nil {
		t.Fatal("expected the request to be refused, got none")
	}
	if !errors.Is(responseError.ClientError, ErrAmbiguousCredentials) {
		t.Errorf("expected the ambiguity to be named, got %v", responseError.ClientError)
	}
	// Nothing is returned beside the refusal, so a caller reading both cannot proceed on an
	// identity that was never chosen.
	if result != nil {
		t.Errorf("expected no identity, got %v", result)
	}
}

// TestExclusiveAdmitsExactlyOne holds that the option refuses ambiguity rather than plurality: an
// endpoint reachable two ways is still reachable, by one of them at a time.
func TestExclusiveAdmitsExactlyOne(t *testing.T) {
	t.Parallel()

	parser := New(
		[]request_parser.RequestParser[any]{
			&refusingParser{},
			&admittingParser{identity: "service account"},
		},
		race_request_parser_config.WithExclusive(),
	)

	result, responseError := parser.Parse(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	if responseError != nil {
		t.Fatalf("unexpected response error: %+v", responseError)
	}
	if result != "service account" {
		t.Errorf("expected the one that admitted it, got %v", result)
	}
}

// TestWithoutExclusiveTwoCredentialsAreAdmitted holds the behaviour every existing caller has, which
// the option does not change: the first parser in declaration order that admitted the request wins.
func TestWithoutExclusiveTwoCredentialsAreAdmitted(t *testing.T) {
	t.Parallel()

	parser := New([]request_parser.RequestParser[any]{
		&admittingParser{identity: "session"},
		&admittingParser{identity: "service account"},
	})

	result, responseError := parser.Parse(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	if responseError != nil {
		t.Fatalf("unexpected response error: %+v", responseError)
	}
	if result == nil {
		t.Error("expected the request to be admitted")
	}
}
