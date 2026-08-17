package list_role_assignments_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	UserKey      string
	RoleId       string
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

func WithUserKey(userKey string) Option {
	return func(config *Config) {
		config.UserKey = userKey
	}
}

func WithRoleId(roleId string) Option {
	return func(config *Config) {
		config.RoleId = roleId
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
