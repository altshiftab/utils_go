package fetch_config

import (
	"maps"
	"net/http"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config/retry_config"
)

type Option func(*Config)

const (
	DefaultMethod = "GET"
)

type Config struct {
	Method               string
	Headers              map[string]string
	Body                 []byte
	SkipReadResponseBody bool
	SkipErrorOnStatus    bool
	RetryConfig          *retry_config.Config
	HttpClient           *http.Client
}

func New(options ...Option) *Config {
	config := &Config{
		Method:     DefaultMethod,
		HttpClient: http.DefaultClient,
	}

	for _, option := range options {
		if option != nil {
			option(config)
		}
	}

	return config
}

func WithMethod(method string) Option {
	return func(configuration *Config) {
		configuration.Method = method
	}
}

// WithHeaders merges the entries into the configuration's headers, overwriting
// existing values for the same names. Merging (rather than replacing the map)
// lets header options compose: a client's own header option (e.g. a content
// type) must not discard a caller's (e.g. an authorization).
func WithHeaders(headers map[string]string) Option {
	return func(configuration *Config) {
		if configuration.Headers == nil {
			configuration.Headers = make(map[string]string, len(headers))
		}
		maps.Copy(configuration.Headers, headers)
	}
}

func WithBody(body []byte) Option {
	return func(configuration *Config) {
		configuration.Body = body
	}
}

func WithSkipReadResponseBody(skipReadResponseBody bool) Option {
	return func(configuration *Config) {
		configuration.SkipReadResponseBody = skipReadResponseBody
	}
}

func WithSkipErrorOnStatus(skipErrorOnStatus bool) Option {
	return func(configuration *Config) {
		configuration.SkipErrorOnStatus = skipErrorOnStatus
	}
}

func WithRetryConfig(retryConfig *retry_config.Config) Option {
	return func(configuration *Config) {
		configuration.RetryConfig = retryConfig
	}
}

func WithHttpClient(httpClient *http.Client) Option {
	return func(configuration *Config) {
		configuration.HttpClient = httpClient
	}
}
