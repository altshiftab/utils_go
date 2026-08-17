package internal

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	muxTypesResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	responseWriterPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/response_writer"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
)

var errServerFailure = errors.New("server failure")

func newResponseWriter(recorder *httptest.ResponseRecorder) *responseWriterPkg.ResponseWriter {
	return &responseWriterPkg.ResponseWriter{ResponseWriter: recorder}
}

func TestDefaultResponseErrorHandler(t *testing.T) {
	t.Parallel()

	t.Run("nil response error is a no-op", func(t *testing.T) {
		t.Parallel()
		recorder := httptest.NewRecorder()
		DefaultResponseErrorHandler(t.Context(), nil, newResponseWriter(recorder))
		if recorder.Code != http.StatusOK || recorder.Body.Len() != 0 {
			t.Fatalf("expected no write, got code %d body %q", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("nil response writer does not panic", func(t *testing.T) {
		t.Parallel()
		DefaultResponseErrorHandler(t.Context(), &muxTypesResponseError.ResponseError{ServerError: errServerFailure}, nil)
	})

	t.Run("client error writes its status", func(t *testing.T) {
		t.Parallel()
		recorder := httptest.NewRecorder()
		DefaultResponseErrorHandler(
			t.Context(),
			&muxTypesResponseError.ResponseError{ProblemDetail: problem_detail.New(http.StatusBadRequest)},
			newResponseWriter(recorder),
		)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", recorder.Code)
		}
		if recorder.Body.Len() == 0 {
			t.Error("expected a problem detail body")
		}
	})

	t.Run("server error defaults to 500", func(t *testing.T) {
		t.Parallel()
		recorder := httptest.NewRecorder()
		DefaultResponseErrorHandler(
			t.Context(),
			&muxTypesResponseError.ResponseError{ServerError: errServerFailure},
			newResponseWriter(recorder),
		)
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", recorder.Code)
		}
	})

	t.Run("invalid response error writes nothing", func(t *testing.T) {
		t.Parallel()
		recorder := httptest.NewRecorder()
		DefaultResponseErrorHandler(t.Context(), &muxTypesResponseError.ResponseError{}, newResponseWriter(recorder))
		if recorder.Code != http.StatusOK || recorder.Body.Len() != 0 {
			t.Fatalf("expected no write, got code %d", recorder.Code)
		}
	})

	t.Run("already-written header is left untouched", func(t *testing.T) {
		t.Parallel()
		recorder := httptest.NewRecorder()
		writer := newResponseWriter(recorder)
		writer.WriteHeader(http.StatusTeapot)
		DefaultResponseErrorHandler(
			t.Context(),
			&muxTypesResponseError.ResponseError{ProblemDetail: problem_detail.New(http.StatusBadRequest)},
			writer,
		)
		if recorder.Code != http.StatusTeapot {
			t.Fatalf("expected the pre-written 418, got %d", recorder.Code)
		}
	})
}

// TestDefaultDoneCallback verifies that a response served is logged at debug level, and that a
// logger set above it is not asked to build a record it would drop.
//
//nolint:paralleltest // The cases set the process-wide default logger, so they run one at a time.
func TestDefaultDoneCallback(t *testing.T) {
	testCases := []struct {
		name     string
		level    slog.Level
		expected bool
	}{
		{name: "debug", level: slog.LevelDebug, expected: true},
		{name: "info", level: slog.LevelInfo},
		{name: "error", level: slog.LevelError},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var buffer bytes.Buffer

			previous := slog.Default()
			t.Cleanup(func() { slog.SetDefault(previous) })
			slog.SetDefault(
				slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: testCase.level})),
			)

			DefaultDoneCallback(t.Context())

			logged := strings.Contains(buffer.String(), ResponseServedMessage)
			if logged != testCase.expected {
				t.Errorf("logged: got %t, want %t (%q)", logged, testCase.expected, buffer.String())
			}

			if !testCase.expected {
				return
			}

			for _, expected := range []string{`"action":"http_response_served"`, `"level":"DEBUG"`} {
				if !strings.Contains(buffer.String(), expected) {
					t.Errorf("the record lacks %s: %s", expected, buffer.String())
				}
			}
		})
	}
}
