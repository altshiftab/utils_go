package internal

import (
	"net/http"
	"net/http/httptest"
	"testing"

	muxTypesResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
)

func TestDefaultResponseErrorHandler_EffectiveProblemDetailError(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	// Both a client and a server error make GetEffectiveProblemDetail unusable (Type() is
	// ServerError, so the handler reaches the effective-problem-detail step and logs).
	responseError := &muxTypesResponseError.ResponseError{ClientError: errServerFailure, ServerError: errServerFailure}
	DefaultResponseErrorHandler(t.Context(), responseError, newResponseWriter(recorder))

	if recorder.Code != http.StatusOK || recorder.Body.Len() != 0 {
		t.Fatalf("expected no response to be written, got code %d", recorder.Code)
	}
}

func TestDefaultResponseErrorHandler_MakeResponseError(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	// A client error with a zero-status problem detail makes MakeResponse fail.
	responseError := &muxTypesResponseError.ResponseError{ClientError: errServerFailure, ProblemDetail: &problem_detail.Detail{}}
	DefaultResponseErrorHandler(t.Context(), responseError, newResponseWriter(recorder))

	if recorder.Code != http.StatusOK || recorder.Body.Len() != 0 {
		t.Fatalf("expected no response to be written, got code %d", recorder.Code)
	}
}
