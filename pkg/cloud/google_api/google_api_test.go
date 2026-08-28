package google_api

import (
	"errors"
	"net/http"
	"testing"
)

var errBoom = errors.New("boom")

func TestIsRateLimited(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		response     *http.Response
		responseBody string
		want         bool
	}{
		{
			name:     "too many requests",
			response: &http.Response{StatusCode: http.StatusTooManyRequests},
			want:     true,
		},
		{
			// The case the status code cannot tell on its own, and the reason
			// the checker is given the body at all.
			name:         "forbidden for asking too often",
			response:     &http.Response{StatusCode: http.StatusForbidden},
			responseBody: `{"error":{"code":403,"errors":[{"domain":"usageLimits","reason":"userRateLimitExceeded"}],"status":"PERMISSION_DENIED"}}`,
			want:         true,
		},
		{
			name:         "forbidden by the newer spelling",
			response:     &http.Response{StatusCode: http.StatusForbidden},
			responseBody: `{"error":{"code":403,"status":"RESOURCE_EXHAUSTED"}}`,
			want:         true,
		},
		{
			name:         "quota for the day",
			response:     &http.Response{StatusCode: http.StatusForbidden},
			responseBody: `{"error":{"errors":[{"reason":"dailyLimitExceeded"}]}}`,
			want:         true,
		},
		{
			// Refused for want of permission, which will be refused again: the
			// delegation not granted, the scope not the one granted.
			name:         "forbidden for want of permission",
			response:     &http.Response{StatusCode: http.StatusForbidden},
			responseBody: `{"error":{"code":403,"errors":[{"domain":"global","reason":"forbidden"}],"status":"PERMISSION_DENIED"}}`,
			want:         false,
		},
		{
			name:     "forbidden with no body to read",
			response: &http.Response{StatusCode: http.StatusForbidden},
			want:     false,
		},
		{
			name:         "forbidden with a body that is not JSON",
			response:     &http.Response{StatusCode: http.StatusForbidden},
			responseBody: "<html>no</html>",
			want:         false,
		},
		{
			name:     "not found",
			response: &http.Response{StatusCode: http.StatusNotFound},
			want:     false,
		},
		{
			name:     "success",
			response: &http.Response{StatusCode: http.StatusOK},
			want:     false,
		},
		{
			name: "no response at all",
			want: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := IsRateLimited(testCase.response, []byte(testCase.responseBody)); got != testCase.want {
				t.Errorf("IsRateLimited() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestRetryResponseChecker(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		response     *http.Response
		responseBody string
		err          error
		want         bool
	}{
		{
			name:         "rate limited in the body",
			response:     &http.Response{StatusCode: http.StatusForbidden},
			responseBody: `{"error":{"errors":[{"reason":"rateLimitExceeded"}]}}`,
			want:         true,
		},
		{
			name:     "server error",
			response: &http.Response{StatusCode: http.StatusInternalServerError},
			want:     true,
		},
		{
			name:     "too many requests",
			response: &http.Response{StatusCode: http.StatusTooManyRequests},
			want:     true,
		},
		{
			// Nothing came back: the network's to have caused, and it may not
			// happen again.
			name: "no response, an error",
			err:  errBoom,
			want: true,
		},
		{
			name: "no response and no error",
			want: false,
		},
		{
			name:         "refused outright",
			response:     &http.Response{StatusCode: http.StatusForbidden},
			responseBody: `{"error":{"errors":[{"reason":"forbidden"}]}}`,
			want:         false,
		},
		{
			name:     "gone",
			response: &http.Response{StatusCode: http.StatusNotFound},
			want:     false,
		},
		{
			name:     "success",
			response: &http.Response{StatusCode: http.StatusOK},
			want:     false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := RetryResponseChecker.Check(testCase.response, []byte(testCase.responseBody), testCase.err)
			if got != testCase.want {
				t.Errorf("Check() = %v, want %v", got, testCase.want)
			}
		})
	}
}
