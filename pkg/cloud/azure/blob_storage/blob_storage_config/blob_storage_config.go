package blob_storage_config

import (
	"net/url"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	// BaseUrl replaces the account's own endpoint, https://<account>.blob.core.windows.net.
	BaseUrl *url.URL
	// FetchOptions are sent with every request. Authenticate through the client set with
	// fetch_config.WithHttpClient, whose transport adds an Entra token for
	// entra.ScopeAzureStorage: the client signs nothing with an account key.
	FetchOptions []fetch_config.Option
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

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
