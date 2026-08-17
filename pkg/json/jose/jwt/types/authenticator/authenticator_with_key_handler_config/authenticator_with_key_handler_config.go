package authenticator_with_key_handler_config

import (
	"github.com/altshiftab/utils_go/pkg/interfaces/validator"
)

type Config struct {
	ClaimsValidator validator.Validator[map[string]any]
	HeaderValidator validator.Validator[map[string]any]
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{}
	for _, option := range options {
		option(config)
	}

	return config
}

func WithClaimsValidator(claimsValidator validator.Validator[map[string]any]) Option {
	return func(config *Config) {
		config.ClaimsValidator = claimsValidator
	}
}

func WithHeaderValidator(headerValidator validator.Validator[map[string]any]) Option {
	return func(config *Config) {
		config.HeaderValidator = headerValidator
	}
}
