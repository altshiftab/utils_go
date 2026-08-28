package list_messages_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	// Query narrows what is listed, in the same format as the Gmail search box
	// ("in:inbox", "from:user@example.com"). Empty lists everything.
	Query string
	// LabelIds narrows what is listed to messages carrying all of them.
	LabelIds []string
	// IncludeSpamTrash lists what is in spam and trash too, which Gmail leaves
	// out otherwise.
	IncludeSpamTrash bool
	// MaxResults is how many are asked for per page. Zero takes Gmail's own
	// default, which is a hundred; the most it serves is five hundred, and a
	// listing that is to be read whole is cheaper in fewer, larger pages.
	MaxResults   int
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

func WithLabelIds(labelIds ...string) Option {
	return func(config *Config) {
		config.LabelIds = append(config.LabelIds, labelIds...)
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

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
