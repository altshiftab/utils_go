package mux

import (
	"net/http"
	"testing"

	muxTypesResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
)

// checkResult asserts the shape of a ResponseError: wantServerError expects a
// ServerError, wantStatus == 0 expects a nil result, and a positive wantStatus
// expects a ProblemDetail with that status.
func checkResult(t *testing.T, result *muxTypesResponseError.ResponseError, wantStatus int, wantServerError bool) {
	t.Helper()

	switch {
	case wantServerError:
		if result == nil || result.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", result)
		}
	case wantStatus == 0:
		if result != nil {
			t.Fatalf("expected nil result, got %#v", result)
		}
	default:
		if result == nil || result.ProblemDetail == nil {
			t.Fatalf("expected a problem detail, got %#v", result)
		}
		if result.ProblemDetail.Status != wantStatus {
			t.Fatalf("expected status %d, got %d", wantStatus, result.ProblemDetail.Status)
		}
	}
}

func TestHandleFetchMetadata(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		header          http.Header
		method          string
		wantStatus      int
		wantServerError bool
	}{
		{name: "nil header", header: nil, method: http.MethodGet, wantServerError: true},
		{name: "empty method", header: http.Header{}, method: "", wantServerError: true},
		{name: "no fetch site", header: http.Header{}, method: http.MethodGet},
		{name: "same-origin", header: http.Header{"Sec-Fetch-Site": {"same-origin"}}, method: http.MethodGet},
		{name: "same-site", header: http.Header{"Sec-Fetch-Site": {"same-site"}}, method: http.MethodGet},
		{name: "none", header: http.Header{"Sec-Fetch-Site": {"none"}}, method: http.MethodGet},
		{
			name: "cross-site top-level navigation",
			header: http.Header{
				"Sec-Fetch-Site": {"cross-site"},
				"Sec-Fetch-Mode": {"navigate"},
				"Sec-Fetch-Dest": {"document"},
			},
			method: http.MethodGet,
		},
		{
			name: "cross-site top-level navigation with empty dest",
			header: http.Header{
				"Sec-Fetch-Site": {"cross-site"},
				"Sec-Fetch-Mode": {"navigate"},
				"Sec-Fetch-Dest": {"empty"},
			},
			method: http.MethodGet,
		},
		{
			name:       "cross-site navigation but not GET",
			header:     http.Header{"Sec-Fetch-Site": {"cross-site"}, "Sec-Fetch-Mode": {"navigate"}, "Sec-Fetch-Dest": {"document"}},
			method:     http.MethodPost,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "cross-site iframe navigation",
			header:     http.Header{"Sec-Fetch-Site": {"cross-site"}, "Sec-Fetch-Mode": {"navigate"}, "Sec-Fetch-Dest": {"iframe"}},
			method:     http.MethodGet,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "cross-site frame navigation",
			header:     http.Header{"Sec-Fetch-Site": {"cross-site"}, "Sec-Fetch-Mode": {"navigate"}, "Sec-Fetch-Dest": {"frame"}},
			method:     http.MethodGet,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "cross-site non-navigation",
			header:     http.Header{"Sec-Fetch-Site": {"cross-site"}, "Sec-Fetch-Mode": {"cors"}, "Sec-Fetch-Dest": {"empty"}},
			method:     http.MethodGet,
			wantStatus: http.StatusForbidden,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			checkResult(t, HandleFetchMetadata(testCase.header, testCase.method), testCase.wantStatus, testCase.wantServerError)
		})
	}
}

func TestValidateContentType(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		expected        string
		header          http.Header
		wantStatus      int
		wantServerError bool
	}{
		{name: "no expectation", expected: "", header: http.Header{}},
		{name: "nil header", expected: "application/json", header: nil, wantServerError: true},
		{
			name:       "missing content type",
			expected:   "application/json",
			header:     http.Header{},
			wantStatus: http.StatusUnsupportedMediaType,
		},
		{
			name:     "matching content type",
			expected: "application/json",
			header:   http.Header{"Content-Type": {"application/json"}},
		},
		{
			name:     "matching content type with parameter",
			expected: "application/json",
			header:   http.Header{"Content-Type": {"application/json; charset=utf-8"}},
		},
		{
			name:       "mismatched content type",
			expected:   "application/json",
			header:     http.Header{"Content-Type": {"text/plain"}},
			wantStatus: http.StatusUnsupportedMediaType,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			checkResult(t, ValidateContentType(testCase.expected, testCase.header), testCase.wantStatus, testCase.wantServerError)
		})
	}
}

func TestValidateContentLength(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		allowEmpty      bool
		header          http.Header
		wantStatus      int
		wantServerError bool
	}{
		{name: "nil header", header: nil, wantServerError: true},
		{name: "missing length not allowed", allowEmpty: false, header: http.Header{}, wantStatus: http.StatusLengthRequired},
		{name: "missing length allowed", allowEmpty: true, header: http.Header{}},
		{
			name:       "zero length not allowed",
			header:     http.Header{"Content-Length": {"0"}},
			wantStatus: http.StatusBadRequest,
		},
		{name: "zero length allowed", allowEmpty: true, header: http.Header{"Content-Length": {"0"}}},
		{name: "positive length", header: http.Header{"Content-Length": {"5"}}},
		{
			name:       "malformed length",
			header:     http.Header{"Content-Length": {"not-a-number"}},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			checkResult(t, ValidateContentLength(testCase.allowEmpty, testCase.header), testCase.wantStatus, testCase.wantServerError)
		})
	}
}
