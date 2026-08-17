package retry_config

import (
	"net/http"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config/retry_config/response_checker"
)

func TestNewDefaults(t *testing.T) {
	t.Parallel()

	config := New()

	if config.Count != DefaultCount {
		t.Errorf("Count = %d, want %d", config.Count, DefaultCount)
	}
	if config.BaseDelay != DefaultBaseDelay {
		t.Errorf("BaseDelay = %v, want %v", config.BaseDelay, DefaultBaseDelay)
	}
	if config.MaximumWaitTime != 0 {
		t.Errorf("MaximumWaitTime = %v, want 0", config.MaximumWaitTime)
	}
	if config.ResponseChecker == nil {
		t.Error("ResponseChecker is nil, want default")
	}
	if config.RetryAfterFunc != nil {
		t.Error("RetryAfterFunc is non-nil, want nil")
	}
}

func TestNewNilOptionSkipped(t *testing.T) {
	t.Parallel()

	config := New(nil, WithCount(5), nil)
	if config.Count != 5 {
		t.Errorf("Count = %d, want 5", config.Count)
	}
}

func TestNewOptions(t *testing.T) {
	t.Parallel()

	checker := response_checker.New(func(*http.Response, error) bool { return false })
	retryAfter := func(*http.Response, []byte) *time.Duration { return nil }

	config := New(
		WithCount(7),
		WithBaseDelay(time.Second),
		WithMaximumWaitTime(10*time.Second),
		WithResponseChecker(checker),
		WithRetryAfterFunc(retryAfter),
	)

	if config.Count != 7 {
		t.Errorf("Count = %d, want 7", config.Count)
	}
	if config.BaseDelay != time.Second {
		t.Errorf("BaseDelay = %v, want %v", config.BaseDelay, time.Second)
	}
	if config.MaximumWaitTime != 10*time.Second {
		t.Errorf("MaximumWaitTime = %v, want %v", config.MaximumWaitTime, 10*time.Second)
	}
	if config.ResponseChecker == nil {
		t.Error("ResponseChecker is nil, want the provided checker")
	}
	if config.RetryAfterFunc == nil {
		t.Error("RetryAfterFunc is nil, want the provided func")
	}
}

func TestDefaultResponseChecker(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		response *http.Response
		err      error
		want     bool
	}{
		{name: "too many requests", response: &http.Response{StatusCode: http.StatusTooManyRequests}, want: true},
		{name: "internal server error", response: &http.Response{StatusCode: http.StatusInternalServerError}, want: true},
		{name: "bad gateway", response: &http.Response{StatusCode: http.StatusBadGateway}, want: true},
		{name: "ok", response: &http.Response{StatusCode: http.StatusOK}, want: false},
		{name: "client error not retried", response: &http.Response{StatusCode: http.StatusBadRequest}, want: false},
		{name: "nil response with error", response: nil, err: http.ErrHandlerTimeout, want: true},
		{name: "nil response no error", response: nil, err: nil, want: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := DefaultResponseChecker.Check(testCase.response, testCase.err); got != testCase.want {
				t.Errorf("Check() = %v, want %v", got, testCase.want)
			}
		})
	}
}
