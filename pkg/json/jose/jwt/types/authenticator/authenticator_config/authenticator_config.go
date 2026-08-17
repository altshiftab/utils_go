package authenticator_config

import (
	motmedelCryptoInterfaces "github.com/altshiftab/utils_go/pkg/crypto/interfaces"
	"github.com/altshiftab/utils_go/pkg/interfaces/validator"
)

type Config struct {
	SignatureVerifier motmedelCryptoInterfaces.NamedVerifier
	ClaimsValidator   validator.Validator[map[string]any]
	HeaderValidator   validator.Validator[map[string]any]
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{}
	for _, option := range options {
		option(config)
	}

	return config
}

func WithSignatureVerifier(signatureVerifier motmedelCryptoInterfaces.NamedVerifier) Option {
	return func(config *Config) {
		config.SignatureVerifier = signatureVerifier
	}
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
