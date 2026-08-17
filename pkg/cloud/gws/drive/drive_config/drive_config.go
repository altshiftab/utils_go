package drive_config

import (
	"net/url"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	BaseUrl           *url.URL
	SupportsAllDrives bool
	FetchOptions      []fetch_config.Option
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{}
	for _, option := range options {
		option(config)
	}

	return config
}

func WithBaseUrl(baseUrl *url.URL) Option {
	return func(config *Config) {
		config.BaseUrl = baseUrl
	}
}

// WithSupportsAllDrives makes the client send supportsAllDrives=true on every
// request, marking the application as handling both My Drive and shared drive
// items.
func WithSupportsAllDrives(supportsAllDrives bool) Option {
	return func(config *Config) {
		config.SupportsAllDrives = supportsAllDrives
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
