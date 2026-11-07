package reader_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

// DefaultMaxReconnects bounds how many times a read broken off midway is
// resumed.
const DefaultMaxReconnects = 8

type Config struct {
	// Generation pins the object generation to read. Zero reads the live one,
	// and pins whatever generation that turns out to be.
	Generation int64
	// Offset is the first byte read.
	Offset int64
	// Length is how many bytes are read from Offset; zero or less reads to the
	// end of the object.
	Length        int64
	MaxReconnects int
	FetchOptions  []fetch_config.Option
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{MaxReconnects: DefaultMaxReconnects}
	for _, option := range options {
		if option != nil {
			option(config)
		}
	}

	return config
}

func WithGeneration(generation int64) Option {
	return func(config *Config) {
		config.Generation = generation
	}
}

func WithRange(offset int64, length int64) Option {
	return func(config *Config) {
		config.Offset = offset
		config.Length = length
	}
}

func WithMaxReconnects(maxReconnects int) Option {
	return func(config *Config) {
		config.MaxReconnects = maxReconnects
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
