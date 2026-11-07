package upload_config

import (
	"time"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

const (
	// ChunkGranularity is the unit Cloud Storage keeps a resumable upload in:
	// every chunk but the last must be a whole number of them.
	ChunkGranularity = 256 << 10
	// DefaultChunkSize is what one request carries.
	DefaultChunkSize = 64 * ChunkGranularity
	// DefaultMaxAttempts bounds the attempts made in a row without the upload
	// getting any further.
	DefaultMaxAttempts = 8
	// DefaultRetryBaseDelay is the wait before the second attempt; each one
	// after that waits twice as long, up to DefaultRetryMaxDelay.
	DefaultRetryBaseDelay = time.Second
	DefaultRetryMaxDelay  = 32 * time.Second
)

type Config struct {
	// IfAbsent makes the upload create the object only where the name is free,
	// and report a taken name as nothing created rather than as an error.
	IfAbsent bool
	// ChunkSize is how many bytes one request carries, a multiple of
	// ChunkGranularity.
	ChunkSize int64
	// SessionUri resumes a session opened earlier, by this process or another,
	// instead of opening a new one.
	SessionUri string
	// OnSession is told the URI of a session as soon as it is opened, so that a
	// caller can keep it and resume the upload in a later process.
	OnSession      func(sessionUri string)
	MaxAttempts    int
	RetryBaseDelay time.Duration
	RetryMaxDelay  time.Duration
	FetchOptions   []fetch_config.Option
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{
		ChunkSize:      DefaultChunkSize,
		MaxAttempts:    DefaultMaxAttempts,
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

func WithIfAbsent(ifAbsent bool) Option {
	return func(config *Config) {
		config.IfAbsent = ifAbsent
	}
}

func WithChunkSize(chunkSize int64) Option {
	return func(config *Config) {
		config.ChunkSize = chunkSize
	}
}

func WithSessionUri(sessionUri string) Option {
	return func(config *Config) {
		config.SessionUri = sessionUri
	}
}

func WithOnSession(onSession func(sessionUri string)) Option {
	return func(config *Config) {
		config.OnSession = onSession
	}
}

func WithMaxAttempts(maxAttempts int) Option {
	return func(config *Config) {
		config.MaxAttempts = maxAttempts
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
