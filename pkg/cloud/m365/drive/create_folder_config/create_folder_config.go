package create_folder_config

import (
	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/types/conflict_behavior"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	// ConflictBehavior defaults to conflict_behavior.Fail.
	ConflictBehavior conflict_behavior.ConflictBehavior
	FetchOptions     []fetch_config.Option
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{ConflictBehavior: conflict_behavior.Fail}
	for _, option := range options {
		option(config)
	}

	return config
}

func WithConflictBehavior(conflictBehavior conflict_behavior.ConflictBehavior) Option {
	return func(config *Config) {
		config.ConflictBehavior = conflictBehavior
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
