// Package id_token_request_parser_config holds the settings of a Google ID token request parser.
package id_token_request_parser_config

import "net/url"

type Config struct {
	// Audience is what the token must have been minted for. It is checked because a token minted
	// for another service would otherwise be accepted here: anyone can point a caller at a public
	// endpoint, and the audience is what says the token was meant for this one.
	Audience string

	// ServiceAccountEmails are the callers admitted. Any Google account can mint a token for a
	// public endpoint, so the audience alone says a token was meant for this service rather than
	// that it came from something entitled to call it.
	//
	// Several are allowed because a service is commonly called by more than one -- a scheduler and
	// a queue, say -- and racing two parsers that differ only in an address would be a strange way
	// to say so.
	ServiceAccountEmails []string

	// JwkUrl is where the signing keys are published. A nil one is Google's.
	JwkUrl *url.URL
}

type Option func(*Config)

// New builds a config from the options. A nil option is skipped, so a caller can pass one
// conditionally without guarding the call.
func New(options ...Option) *Config {
	config := &Config{}
	for _, option := range options {
		if option != nil {
			option(config)
		}
	}

	return config
}

func WithAudience(audience string) Option {
	return func(config *Config) {
		config.Audience = audience
	}
}

// WithServiceAccountEmails adds to what an earlier call set rather than replacing it.
func WithServiceAccountEmails(emails ...string) Option {
	return func(config *Config) {
		config.ServiceAccountEmails = append(config.ServiceAccountEmails, emails...)
	}
}

func WithJwkUrl(jwkUrl *url.URL) Option {
	return func(config *Config) {
		config.JwkUrl = jwkUrl
	}
}
