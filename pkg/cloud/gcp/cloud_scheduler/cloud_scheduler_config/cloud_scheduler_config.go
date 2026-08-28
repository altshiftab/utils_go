// Package cloud_scheduler_config holds the settings of a Cloud Scheduler client.
package cloud_scheduler_config

import (
	"net/url"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	// BaseUrl is the endpoint the requests are sent to. A nil one is the public API.
	BaseUrl *url.URL

	// FetchOptions are passed to every request the client makes, which is where a caller puts its
	// own HTTP client, timeout or credentials.
	FetchOptions []fetch_config.Option
}

type Option func(*Config)

// New builds a config from the options. A nil option is skipped, so a caller can pass one
// conditionally without guarding the call.
func New(options ...Option) *Config {
	config := &Config{}
	for _, option := range options {
		if option != nil {
			option(config)
		}
	}

	return config
}

func WithBaseUrl(baseUrl *url.URL) Option {
	return func(config *Config) {
		config.BaseUrl = baseUrl
	}
}

// WithFetchOptions adds to what an earlier call set rather than replacing it.
func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
