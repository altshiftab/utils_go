package response_checker

import (
	"errors"
	"net/http"
	"testing"
)

var errBoom = errors.New("boom")

func TestResponseCheckerFunctionCheck(t *testing.T) {
	t.Parallel()

	var gotResponse *http.Response
	var gotBody []byte
	var gotErr error

	sentinelResponse := &http.Response{StatusCode: http.StatusTeapot}
	sentinelBody := []byte(`{"error":{"status":"RESOURCE_EXHAUSTED"}}`)
	sentinelErr := errBoom

	checker := ResponseCheckerFunction(func(response *http.Response, responseBody []byte, err error) bool {
		gotResponse = response
		gotBody = responseBody
		gotErr = err
		return response != nil
	})

	if !checker.Check(sentinelResponse, sentinelBody, sentinelErr) {
		t.Error("Check() = false, want true")
	}
	if gotResponse != sentinelResponse {
		t.Errorf("response passed = %p, want %p", gotResponse, sentinelResponse)
	}
	// The body reaching the checker is the point of it being there: a refusal
	// Google spells out in the body cannot be read from the status alone.
	if string(gotBody) != string(sentinelBody) {
		t.Errorf("body passed = %q, want %q", gotBody, sentinelBody)
	}
	if !errors.Is(gotErr, sentinelErr) {
		t.Errorf("err passed = %v, want %v", gotErr, sentinelErr)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		fn           func(*http.Response, []byte, error) bool
		response     *http.Response
		responseBody []byte
		err          error
		want         bool
	}{
		{
			name:     "always true",
			fn:       func(*http.Response, []byte, error) bool { return true },
			response: &http.Response{},
			want:     true,
		},
		{
			name: "based on status code",
			fn: func(response *http.Response, _ []byte, _ error) bool {
				return response != nil && response.StatusCode >= 500
			},
			response: &http.Response{StatusCode: http.StatusServiceUnavailable},
			want:     true,
		},
		{
			name: "based on status code below threshold",
			fn: func(response *http.Response, _ []byte, _ error) bool {
				return response != nil && response.StatusCode >= 500
			},
			response: &http.Response{StatusCode: http.StatusOK},
			want:     false,
		},
		{
			name: "based on the body, which the status alone does not tell",
			fn: func(_ *http.Response, responseBody []byte, _ error) bool {
				return len(responseBody) != 0
			},
			response:     &http.Response{StatusCode: http.StatusForbidden},
			responseBody: []byte(`{"error":{"status":"RESOURCE_EXHAUSTED"}}`),
			want:         true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			checker := New(testCase.fn)
			if checker == nil {
				t.Fatal("New() returned nil")
			}
			if got := checker.Check(testCase.response, testCase.responseBody, testCase.err); got != testCase.want {
				t.Errorf("Check() = %v, want %v", got, testCase.want)
			}
		})
	}
}
