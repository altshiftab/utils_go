package drive_config

import (
	"net/http"
	"net/url"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	BaseUrl      *url.URL
	FetchOptions []fetch_config.Option
	// UploadHttpClient sends the chunks of an upload session. The session URL
	// is pre-authenticated and Graph rejects a bearer token sent to it, so this
	// must not be the authenticating client; nil means http.DefaultClient.
	// Authenticate through the client set with fetch_config.WithHttpClient,
	// never an Authorization header in FetchOptions, which the chunks would
	// carry as well.
	UploadHttpClient *http.Client
	// UploadChunkSize is the upload session chunk size in bytes, rounded down
	// to the multiple of 320 KiB Graph requires (at least one). Zero means the
	// client default.
	UploadChunkSize int
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

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}

func WithUploadHttpClient(uploadHttpClient *http.Client) Option {
	return func(config *Config) {
		config.UploadHttpClient = uploadHttpClient
	}
}

func WithUploadChunkSize(uploadChunkSize int) Option {
	return func(config *Config) {
		config.UploadChunkSize = uploadChunkSize
	}
}
