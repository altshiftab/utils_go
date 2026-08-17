package redirector

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
)

func TestNew_Errors(t *testing.T) {
	t.Parallel()

	redirectUrl, err := url.Parse("https://example.com/login")
	if err != nil {
		t.Fatalf("url parse: %v", err)
	}

	parser := request_parser.New(func(*http.Request) (string, *response_error.ResponseError) {
		return "", nil
	})

	if _, err := New[request_parser.RequestParser[string], string](nil, redirectUrl); err == nil {
		t.Fatal("expected an error for a nil request parser")
	}
	if _, err := New(parser, nil); err == nil {
		t.Fatal("expected an error for a nil redirect url")
	}
}
