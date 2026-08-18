package github_config

import (
	"net/url"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	// BaseUrl is where the API is reached. Defaults to GitHub's own.
	BaseUrl *url.URL
	// ArchiveBaseUrl is where commit archives are reached. It is separate
	// because they are served by the web host rather than the API.
	ArchiveBaseUrl *url.URL
	FetchOptions   []fetch_config.Option
}

type Option func(*Config)

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

func WithArchiveBaseUrl(archiveBaseUrl *url.URL) Option {
	return func(config *Config) {
		config.ArchiveBaseUrl = archiveBaseUrl
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}

// WithToken authenticates requests, which raises the rate limit and is what
// reaching a private repository needs.
func WithToken(token string) Option {
	return func(config *Config) {
		if token == "" {
			return
		}
		config.FetchOptions = append(
			config.FetchOptions,
			fetch_config.WithHeaders(map[string]string{"Authorization": "Bearer " + token}),
		)
	}
}
