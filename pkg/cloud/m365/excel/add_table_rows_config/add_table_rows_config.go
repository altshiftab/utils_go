package add_table_rows_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	// Index is the zero-based position the rows are inserted at; nil appends.
	Index        *int
	SessionId    string
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

func WithIndex(index int) Option {
	return func(config *Config) {
		config.Index = &index
	}
}

func WithSessionId(sessionId string) Option {
	return func(config *Config) {
		config.SessionId = sessionId
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
