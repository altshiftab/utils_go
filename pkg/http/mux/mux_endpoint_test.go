package mux

import (
	"net/http"
	"net/http/httptest"
	"testing"

	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	muxResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	muxResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/schema"
)

type stubUserer struct{ user *schema.User }

func (s stubUserer) GetUser() *schema.User { return s.user }

func anyParser(value any, responseError *muxResponseError.ResponseError) request_parser.RequestParser[any] {
	return request_parser.New(func(*http.Request) (any, *muxResponseError.ResponseError) {
		return value, responseError
	})
}

func okHandlerEndpoint(path string) *endpointPkg.Endpoint {
	return &endpointPkg.Endpoint{
		Path:   path,
		Method: http.MethodGet,
		Public: true,
		Handler: func(*http.Request, []byte) (*muxResponse.Response, *muxResponseError.ResponseError) {
			return &muxResponse.Response{Body: []byte("ok")}, nil
		},
	}
}

func TestMuxHandleRequest_EarlyReturns(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)

	if _, responseError := muxHandleRequest(nil, request, recorder); responseError == nil || responseError.ServerError == nil {
		t.Error("expected a server error for a nil mux")
	}
	if _, responseError := muxHandleRequest(New(), nil, recorder); responseError == nil || responseError.ServerError == nil {
		t.Error("expected a server error for a nil request")
	}
	if _, responseError := muxHandleRequest(New(), &http.Request{}, recorder); responseError == nil || responseError.ServerError == nil {
		t.Error("expected a server error for a nil request header")
	}
	// The request carries no mux http context.
	if _, responseError := muxHandleRequest(New(), request, recorder); responseError == nil || responseError.ServerError == nil {
		t.Error("expected a server error for a missing http context")
	}
}

func TestMux_ServeHTTP_AuthenticationParser(t *testing.T) {
	t.Parallel()

	t.Run("error propagates", func(t *testing.T) {
		t.Parallel()
		authError := &muxResponseError.ResponseError{ProblemDetail: problem_detail.New(http.StatusUnauthorized)}
		mux := New(&endpointPkg.Endpoint{
			Path:                 "/x",
			Method:               http.MethodGet,
			AuthenticationParser: anyParser(nil, authError),
		})
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", recorder.Code)
		}
	})

	t.Run("userer authentication succeeds", func(t *testing.T) {
		t.Parallel()
		endpoint := okHandlerEndpoint("/x")
		endpoint.AuthenticationParser = anyParser(stubUserer{user: &schema.User{Id: "u1"}}, nil)
		mux := New(endpoint)
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("got %d, want 200", recorder.Code)
		}
	})
}

func TestMux_ServeHTTP_UrlAndHeaderParsers(t *testing.T) {
	t.Parallel()

	// Each subtest uses its own ResponseError; the error handler mutates it, so a shared
	// instance would race across the parallel subtests.
	t.Run("url parser error", func(t *testing.T) {
		t.Parallel()
		badRequest := &muxResponseError.ResponseError{ProblemDetail: problem_detail.New(http.StatusBadRequest)}
		mux := New(&endpointPkg.Endpoint{Path: "/x", Method: http.MethodGet, Public: true, UrlParser: anyParser(nil, badRequest)})
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", recorder.Code)
		}
	})

	t.Run("header parser error", func(t *testing.T) {
		t.Parallel()
		badRequest := &muxResponseError.ResponseError{ProblemDetail: problem_detail.New(http.StatusBadRequest)}
		mux := New(&endpointPkg.Endpoint{Path: "/x", Method: http.MethodGet, Public: true, HeaderParser: anyParser(nil, badRequest)})
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", recorder.Code)
		}
	})

	t.Run("both parsers succeed", func(t *testing.T) {
		t.Parallel()
		endpoint := okHandlerEndpoint("/x")
		endpoint.UrlParser = anyParser("url", nil)
		endpoint.HeaderParser = anyParser("header", nil)
		mux := New(endpoint)
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("got %d, want 200", recorder.Code)
		}
	})
}

func TestMux_ServeHTTP_FetchMetadataBlocks(t *testing.T) {
	t.Parallel()

	mux := New(okHandlerEndpoint("/x"))

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	request.Header.Set("Sec-Fetch-Mode", "cors")
	request.Header.Set("Sec-Fetch-Dest", "empty")

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", recorder.Code)
	}
}
