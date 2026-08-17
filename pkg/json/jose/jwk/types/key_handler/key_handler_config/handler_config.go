package key_handler_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	FetchOptions []fetch_config.Option
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{}
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
