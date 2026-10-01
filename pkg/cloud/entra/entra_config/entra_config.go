package entra_config

import (
	"net/url"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	// AuthorityUrl overrides the login host, e.g. for a national cloud.
	AuthorityUrl *url.URL
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

func WithAuthorityUrl(authorityUrl *url.URL) Option {
	return func(config *Config) {
		config.AuthorityUrl = authorityUrl
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
