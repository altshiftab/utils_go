package jwt_extractor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/mismatch_error"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	authenticatorPkg "github.com/altshiftab/utils_go/pkg/interfaces/authenticator"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/token/authenticated_token"
)

var errAuthFailure = errors.New("authenticator failure")

func tokenExtractor(value string, responseError *response_error.ResponseError) request_parser.RequestParser[string] {
	return request_parser.New(func(*http.Request) (string, *response_error.ResponseError) {
		return value, responseError
	})
}

func authenticatorReturning(token *authenticated_token.Token, err error) authenticatorPkg.Authenticator[*authenticated_token.Token, string] {
	return authenticatorPkg.New(func(context.Context, string) (*authenticated_token.Token, error) {
		return token, err
	})
}

func newRequest(t *testing.T) *http.Request {
	t.Helper()
	return httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
}

func TestParse(t *testing.T) {
	t.Parallel()

	t.Run("nil request is a server error", func(t *testing.T) {
		t.Parallel()
		parser, err := New(tokenExtractor("tok", nil))
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		_, responseError := parser.Parse(nil)
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})

	t.Run("nil token extractor is a server error", func(t *testing.T) {
		t.Parallel()
		parser := &Parser[request_parser.RequestParser[string]]{}
		_, responseError := parser.Parse(newRequest(t))
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})

	t.Run("token extractor error propagates", func(t *testing.T) {
		t.Parallel()
		extractorError := &response_error.ResponseError{ProblemDetail: problem_detail.New(http.StatusBadRequest)}
		parser, err := New(tokenExtractor("", extractorError))
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		_, responseError := parser.Parse(newRequest(t))
		if responseError != extractorError {
			t.Fatalf("expected the extractor error, got %#v", responseError)
		}
	})

	t.Run("empty token is 401", func(t *testing.T) {
		t.Parallel()
		parser, err := New(tokenExtractor("", nil))
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		_, responseError := parser.Parse(newRequest(t))
		if responseError == nil || responseError.ProblemDetail == nil || responseError.ProblemDetail.Status != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %#v", responseError)
		}
	})

	t.Run("successful authentication returns the token", func(t *testing.T) {
		t.Parallel()
		token := &authenticated_token.Token{}
		parser, err := New(tokenExtractor("tok", nil), authenticatorReturning(token, nil))
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		result, responseError := parser.Parse(newRequest(t))
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if result != token {
			t.Fatal("expected the authenticated token")
		}
	})

	t.Run("subject mismatch is 403", func(t *testing.T) {
		t.Parallel()
		parser, err := New(tokenExtractor("tok", nil), authenticatorReturning(nil, mismatch_error.New("sub", "expected", "actual")))
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		_, responseError := parser.Parse(newRequest(t))
		if responseError == nil || responseError.ProblemDetail == nil || responseError.ProblemDetail.Status != http.StatusForbidden {
			t.Fatalf("expected 403, got %#v", responseError)
		}
	})

	t.Run("validation error is 401", func(t *testing.T) {
		t.Parallel()
		parser, err := New(tokenExtractor("tok", nil), authenticatorReturning(nil, fmt.Errorf("%w: bad", altshiftErrors.ErrValidationError)))
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		_, responseError := parser.Parse(newRequest(t))
		if responseError == nil || responseError.ProblemDetail == nil || responseError.ProblemDetail.Status != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %#v", responseError)
		}
	})

	t.Run("unknown authenticator error is a server error", func(t *testing.T) {
		t.Parallel()
		parser, err := New(tokenExtractor("tok", nil), authenticatorReturning(nil, errAuthFailure))
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		_, responseError := parser.Parse(newRequest(t))
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})
}

func TestNew_NilExtractor(t *testing.T) {
	t.Parallel()

	if _, err := New[request_parser.RequestParser[string]](nil); err == nil {
		t.Fatal("expected an error for a nil token extractor")
	}
}
