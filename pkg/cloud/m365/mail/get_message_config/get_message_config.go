package get_message_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	// Select names the properties served; empty serves Graph's default set.
	Select []string
	// Expand is an OData expansion, such as "attachments($select=id,name,size)".
	Expand string
	// BodyContentType asks for the body as item_body.ContentTypeText or ContentTypeHtml; empty
	// serves it as it is kept.
	BodyContentType string
	FetchOptions    []fetch_config.Option
}

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

func WithBodyContentType(bodyContentType string) Option {
	return func(config *Config) {
		config.BodyContentType = bodyContentType
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
