package reader_config

import (
	"time"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

const (
	// DefaultMaxReconnects bounds the attempts in a row that bring no bytes:
	// a read broken off, or a request to resume it that fails. Progress resets
	// the count.
	DefaultMaxReconnects = 8
	// DefaultRetryBaseDelay is the wait before the second attempt in a row;
	// each one after that waits twice as long, up to DefaultRetryMaxDelay.
	DefaultRetryBaseDelay = time.Second
	DefaultRetryMaxDelay  = 32 * time.Second
)

type Config struct {
	// Generation pins the object generation to read. Zero reads the live one,
	// and pins whatever generation that turns out to be.
	Generation int64
	// Offset is the first byte read.
	Offset int64
	// Length is how many bytes are read from Offset; zero or less reads to the
	// end of the object.
	Length         int64
	MaxReconnects  int
	RetryBaseDelay time.Duration
	RetryMaxDelay  time.Duration
	FetchOptions   []fetch_config.Option
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{
		MaxReconnects:  DefaultMaxReconnects,
		RetryBaseDelay: DefaultRetryBaseDelay,
		RetryMaxDelay:  DefaultRetryMaxDelay,
	}
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

func WithRetryDelays(baseDelay time.Duration, maxDelay time.Duration) Option {
	return func(config *Config) {
		config.RetryBaseDelay = baseDelay
		config.RetryMaxDelay = maxDelay
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
