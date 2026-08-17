package mux

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/firewall_verdict"
	muxTypesMiddleware "github.com/altshiftab/utils_go/pkg/http/mux/types/middleware"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	muxResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	muxResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
)

var errMissingContextValue = errors.New("missing context value")

type contextTestKey struct{}

type hijackableRecorder struct {
	*httptest.ResponseRecorder
	conn     net.Conn
	hijacked bool
}

func (h *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	return h.conn, bufio.NewReadWriter(bufio.NewReader(h.conn), bufio.NewWriter(h.conn)), nil
}

func firewallMux(verdict firewall_verdict.Verdict, responseError *muxResponseError.ResponseError) *Mux {
	mux := New()
	mux.FirewallParser = request_parser.New(func(*http.Request) (firewall_verdict.Verdict, *muxResponseError.ResponseError) {
		return verdict, responseError
	})
	return mux
}

func TestMux_ServeHTTP_EarlyReturns(t *testing.T) {
	t.Parallel()

	// A nil writer and a nil request both return without panicking.
	New().ServeHTTP(nil, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	New().ServeHTTP(httptest.NewRecorder(), nil)
}

func TestMux_ServeHTTP_FirewallReject(t *testing.T) {
	t.Parallel()

	t.Run("defaults to 403", func(t *testing.T) {
		t.Parallel()
		recorder := httptest.NewRecorder()
		firewallMux(firewall_verdict.Reject, nil).ServeHTTP(
			recorder,
			httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil),
		)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("got %d, want 403", recorder.Code)
		}
	})

	t.Run("uses the provided error", func(t *testing.T) {
		t.Parallel()
		recorder := httptest.NewRecorder()
		customError := &muxResponseError.ResponseError{ProblemDetail: problem_detail.New(http.StatusTeapot)}
		firewallMux(firewall_verdict.Reject, customError).ServeHTTP(
			recorder,
			httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil),
		)
		if recorder.Code != http.StatusTeapot {
			t.Fatalf("got %d, want 418", recorder.Code)
		}
	})
}

func TestMux_ServeHTTP_FirewallDrop(t *testing.T) {
	t.Parallel()

	t.Run("without a hijacker aborts the handler", func(t *testing.T) {
		t.Parallel()
		defer func() {
			recovered := recover()
			if err, ok := recovered.(error); !ok || !errors.Is(err, http.ErrAbortHandler) {
				t.Fatalf("expected an ErrAbortHandler panic, got %v", recovered)
			}
		}()
		firewallMux(firewall_verdict.Drop, nil).ServeHTTP(
			httptest.NewRecorder(),
			httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil),
		)
	})

	t.Run("with a hijacker closes the connection", func(t *testing.T) {
		t.Parallel()
		clientConn, serverConn := net.Pipe()
		defer func() { _ = clientConn.Close() }()

		writer := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder(), conn: serverConn}
		firewallMux(firewall_verdict.Drop, nil).ServeHTTP(
			writer,
			httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil),
		)
		if !writer.hijacked {
			t.Error("expected the connection to be hijacked")
		}
	})
}

func TestMux_ServeHTTP_MiddlewareContextAndDoneCallback(t *testing.T) {
	t.Parallel()

	var middlewareCalled, doneCalled bool

	mux := New(&endpointPkg.Endpoint{
		Path:   "/x",
		Method: http.MethodGet,
		Public: true,
		Handler: func(request *http.Request, _ []byte) (*muxResponse.Response, *muxResponseError.ResponseError) {
			if request.Context().Value(contextTestKey{}) != "value" {
				return nil, &muxResponseError.ResponseError{ServerError: errMissingContextValue}
			}
			return &muxResponse.Response{Body: []byte("ok")}, nil
		},
	})
	mux.Middleware = []muxTypesMiddleware.Middleware{
		func(request *http.Request) *http.Request {
			middlewareCalled = true
			return request
		},
	}
	mux.SetContextKeyValuePairs = [][2]any{{contextTestKey{}, "value"}}
	mux.DoneCallback = func(context.Context) {
		doneCalled = true
	}

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", recorder.Code)
	}
	if recorder.Body.String() != "ok" {
		t.Fatalf("body = %q, want ok", recorder.Body.String())
	}
	if !middlewareCalled {
		t.Error("expected the middleware to run")
	}
	if !doneCalled {
		t.Error("expected the done callback to run")
	}
}
