package url_allower

import (
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/url_allower/url_allower_config"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
)

func urlParser(value string) request_parser.RequestParser[stringURL] {
	return request_parser.New(func(*http.Request) (stringURL, *response_error.ResponseError) {
		return stringURL(value), nil
	})
}

func TestParse_UrlValidation(t *testing.T) {
	t.Parallel()

	t.Run("empty url is 400", func(t *testing.T) {
		t.Parallel()
		_, responseError := New[stringURL](urlParser("")).Parse(&http.Request{})
		if responseError == nil || responseError.ProblemDetail == nil || responseError.ProblemDetail.Status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %#v", responseError)
		}
	})

	t.Run("malformed url is 400", func(t *testing.T) {
		t.Parallel()
		_, responseError := New[stringURL](urlParser("http://[::1")).Parse(&http.Request{})
		if responseError == nil || responseError.ProblemDetail == nil || responseError.ProblemDetail.Status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %#v", responseError)
		}
	})

	t.Run("exact allowed domain match", func(t *testing.T) {
		t.Parallel()
		parser := New[stringURL](
			urlParser("http://app.example.com/"),
			url_allower_config.WithAllowedDomains([]string{"app.example.com"}),
		)
		if _, responseError := parser.Parse(&http.Request{}); responseError != nil {
			t.Fatalf("expected the exact-domain match to be allowed, got %#v", responseError)
		}
	})

	t.Run("nil config is a server error", func(t *testing.T) {
		t.Parallel()
		parser := &Parser[stringURL]{RequestParser: urlParser("http://example.com/")}
		_, responseError := parser.Parse(&http.Request{})
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})
}
