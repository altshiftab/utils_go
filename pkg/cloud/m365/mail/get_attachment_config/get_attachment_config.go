package get_attachment_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	// Select names the properties served; an attachment's content is served unless it is left out.
	Select []string
	// Expand is an OData expansion: ExpandItem serves an item attachment's message.
	Expand       string
	FetchOptions []fetch_config.Option
}

// ExpandItem expands an item attachment's item.
const ExpandItem = "microsoft.graph.itemattachment/item"

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{}
	for _, option := range options {
		option(config)
	}

	return config
}

func WithSelect(properties ...string) Option {
	return func(config *Config) {
		config.Select = append(config.Select, properties...)
	}
}

func WithExpand(expand string) Option {
	return func(config *Config) {
		config.Expand = expand
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
