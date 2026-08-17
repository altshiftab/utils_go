package retry_config

import (
	"net/http"
	"time"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config/retry_config/response_checker"
)

var DefaultResponseChecker = response_checker.New(
	func(response *http.Response, err error) bool {
		if response != nil {
			return response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		}

		if err != nil {
			return true
		}

		return false
	},
)

type Option func(*Config)

const (
	DefaultCount     = 2
	DefaultBaseDelay = time.Duration(500) * time.Millisecond
)

// RetryAfterFunc extracts a server-advised delay before the next attempt from the
// previous response and its body, or returns nil when the response advises none. It
// takes precedence over the Retry-After header and the exponential back-off, letting
// callers honour provider-specific signals such as a google.rpc.RetryInfo in the body.
type RetryAfterFunc func(response *http.Response, responseBody []byte) *time.Duration

type Config struct {
	Count           int
	BaseDelay       time.Duration
	MaximumWaitTime time.Duration
	ResponseChecker response_checker.ResponseChecker
	RetryAfterFunc  RetryAfterFunc
}

func New(options ...Option) *Config {
	config := &Config{
		Count:           DefaultCount,
		BaseDelay:       DefaultBaseDelay,
		ResponseChecker: DefaultResponseChecker,
	}

	for _, option := range options {
		if option != nil {
			option(config)
		}
	}

	return config
}

func WithCount(count int) Option {
	return func(configuration *Config) {
		configuration.Count = count
	}
}

func WithBaseDelay(baseDelay time.Duration) Option {
	return func(configuration *Config) {
		configuration.BaseDelay = baseDelay
	}
}

func WithMaximumWaitTime(maximumWaitTime time.Duration) Option {
	return func(configuration *Config) {
		configuration.MaximumWaitTime = maximumWaitTime
	}
}

func WithResponseChecker(checker response_checker.ResponseChecker) Option {
	return func(configuration *Config) {
		configuration.ResponseChecker = checker
	}
}

func WithRetryAfterFunc(retryAfterFunc RetryAfterFunc) Option {
	return func(configuration *Config) {
		configuration.RetryAfterFunc = retryAfterFunc
	}
}
