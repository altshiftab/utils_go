package list_comments_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	// PageSize is how many comments are asked for per page. Zero takes Drive's own default, which is twenty; the most
	// it serves is a hundred.
	PageSize  int
	PageToken string
	// IncludeDeleted lists deleted comments and replies too, without their content.
	IncludeDeleted bool
	// Fields names what each page returns. Empty takes every field of a comment and its replies that the client reads:
	// Drive answers a listing of comments only with the fields named.
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

func WithPageSize(pageSize int) Option {
	return func(config *Config) {
		config.PageSize = pageSize
	}
}

func WithPageToken(pageToken string) Option {
	return func(config *Config) {
		config.PageToken = pageToken
	}
}

func WithIncludeDeleted(includeDeleted bool) Option {
	return func(config *Config) {
		config.IncludeDeleted = includeDeleted
	}
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
