package move_item_config

import (
	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/types/conflict_behavior"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	// Name renames the item as it moves; empty keeps the current name.
	Name string
	// ConflictBehavior is left to the API default (fail) when empty.
	ConflictBehavior conflict_behavior.ConflictBehavior
	FetchOptions     []fetch_config.Option
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{}
	for _, option := range options {
		option(config)
	}

	return config
}

func WithName(name string) Option {
	return func(config *Config) {
		config.Name = name
	}
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
