package get_file_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

// Config is the options of a call that answers with a file: a read, and the
// create and copy that answer with what they made.
type Config struct {
	// Fields names the file's fields to return, as "id, name, parents". Empty
	// takes Drive's default, which is the id, name and MIME type alone.
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
