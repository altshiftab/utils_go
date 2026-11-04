package list_drafts_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	// Query narrows what is listed, in the same format as the Gmail search box.
	// Empty lists every draft.
	Query string
	// IncludeSpamTrash lists drafts in spam and trash too, which Gmail leaves out
	// otherwise.
	IncludeSpamTrash bool
	// MaxResults is how many are asked for per page. Zero takes Gmail's own
	// default, which is a hundred; the most it serves is five hundred.
	MaxResults int
	// PageToken asks for the page after the one that returned it.
	PageToken    string
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

func WithQuery(query string) Option {
	return func(config *Config) {
		config.Query = query
	}
}

func WithIncludeSpamTrash(includeSpamTrash bool) Option {
	return func(config *Config) {
		config.IncludeSpamTrash = includeSpamTrash
	}
}

func WithMaxResults(maxResults int) Option {
	return func(config *Config) {
		config.MaxResults = maxResults
	}
}

func WithPageToken(pageToken string) Option {
	return func(config *Config) {
		config.PageToken = pageToken
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
