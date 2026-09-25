package key_handler_config

import (
	"time"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

// DefaultUnknownKeyIdRefetchInterval bounds how often a key id missing from the cached set sends the
// handler back to fetch it, so tokens naming made-up key ids cannot turn every request into a fetch.
const DefaultUnknownKeyIdRefetchInterval = time.Minute

type Config struct {
	FetchOptions                []fetch_config.Option
	UnknownKeyIdRefetchInterval time.Duration
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{UnknownKeyIdRefetchInterval: DefaultUnknownKeyIdRefetchInterval}
	for _, option := range options {
		option(config)
	}

	return config
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(configuration *Config) {
		configuration.FetchOptions = append(configuration.FetchOptions, fetchOptions...)
	}
}

// WithUnknownKeyIdRefetchInterval sets the least time between fetches made for a key id the cached
// set does not hold; zero fetches on every such miss.
func WithUnknownKeyIdRefetchInterval(interval time.Duration) Option {
	return func(configuration *Config) {
		configuration.UnknownKeyIdRefetchInterval = interval
	}
}
