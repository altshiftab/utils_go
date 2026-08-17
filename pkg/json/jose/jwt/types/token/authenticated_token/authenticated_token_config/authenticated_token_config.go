package authenticated_token_config

import (
	altshiftCryptoInterfaces "github.com/altshiftab/utils_go/pkg/crypto/interfaces"
	"github.com/altshiftab/utils_go/pkg/interfaces/validator"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/token"
	jwtValidator "github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/validator"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/validator/registered_claims_validator"
)

var DefaultValidator = &jwtValidator.Validator{
	PayloadValidator: &registered_claims_validator.Validator{},
}

type Config struct {
	SignatureVerifier    altshiftCryptoInterfaces.NamedVerifier
	TokenValidator       validator.Validator[*token.Token]
	AllowUnauthenticated bool
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{
		TokenValidator: DefaultValidator,
	}

	for _, option := range options {
		option(config)
	}
	return config
}

func WithSignatureVerifier(signatureVerifier altshiftCryptoInterfaces.NamedVerifier) Option {
	return func(configuration *Config) {
		configuration.SignatureVerifier = signatureVerifier
	}
}

func WithTokenValidator(tokenValidator validator.Validator[*token.Token]) Option {
	return func(configuration *Config) {
		configuration.TokenValidator = tokenValidator
	}
}

func WithAllowUnauthenticated(allowUnauthenticated bool) Option {
	return func(configuration *Config) {
		configuration.AllowUnauthenticated = allowUnauthenticated
	}
}
