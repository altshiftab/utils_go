package get_spreadsheet_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	Fields       string
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

// WithFields sets a response field mask (e.g. "sheets.properties(sheetId,title)").
func WithFields(fields string) Option {
	return func(config *Config) {
		config.Fields = fields
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
