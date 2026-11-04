package update_file_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	// Fields names the file's fields to return, as get_file_config's does.
	Fields string
	// AddParents and RemoveParents move the file: a file's parents are not
	// written as a field, only added to and taken from.
	AddParents    []string
	RemoveParents []string
	FetchOptions  []fetch_config.Option
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

func WithAddParents(addParents ...string) Option {
	return func(config *Config) {
		config.AddParents = append(config.AddParents, addParents...)
	}
}

func WithRemoveParents(removeParents ...string) Option {
	return func(config *Config) {
		config.RemoveParents = append(config.RemoveParents, removeParents...)
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
