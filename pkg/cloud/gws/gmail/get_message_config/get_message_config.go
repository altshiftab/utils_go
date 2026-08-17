package get_message_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Format string

const (
	FormatMinimal  Format = "minimal"
	FormatFull     Format = "full"
	FormatRaw      Format = "raw"
	FormatMetadata Format = "metadata"
)

type Config struct {
	Format          Format
	MetadataHeaders []string
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

func WithFormat(format Format) Option {
	return func(config *Config) {
		config.Format = format
	}
}

func WithMetadataHeaders(metadataHeaders ...string) Option {
	return func(config *Config) {
		config.MetadataHeaders = append(config.MetadataHeaders, metadataHeaders...)
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
