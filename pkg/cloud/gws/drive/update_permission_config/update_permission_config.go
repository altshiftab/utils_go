package update_permission_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	TransferOwnership bool
	RemoveExpiration  bool
	FetchOptions      []fetch_config.Option
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{}
	for _, option := range options {
		option(config)
	}

	return config
}

func WithTransferOwnership(transferOwnership bool) Option {
	return func(config *Config) {
		config.TransferOwnership = transferOwnership
	}
}

// WithRemoveExpiration clears the permission's expiration time.
func WithRemoveExpiration(removeExpiration bool) Option {
	return func(config *Config) {
		config.RemoveExpiration = removeExpiration
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
