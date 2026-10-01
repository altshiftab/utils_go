// Package request_config holds the options every workbook call takes.
package request_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	// SessionId runs the call in a session from CreateSession; empty runs it
	// sessionless.
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
