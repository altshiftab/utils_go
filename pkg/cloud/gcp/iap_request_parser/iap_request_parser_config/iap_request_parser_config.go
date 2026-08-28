// Package iap_request_parser_config holds the settings of an Identity-Aware Proxy request parser.
package iap_request_parser_config

import "net/url"

type Config struct {
	// Audience is what the assertion must have been minted for, which names the resource IAP is in
	// front of. For a load balancer it is "/projects/{number}/global/backendServices/{id}", and for
	// App Engine "/projects/{number}/apps/{id}"; BackendServiceAudience builds the first.
	//
	// It is what stops an assertion minted for another service being accepted here. Every
	// deployment behind IAP is issued assertions by the same IAP, signed with the same keys, so the
	// audience is the only thing distinguishing one service's from another's.
	Audience string

	// AllowedEmails are the accounts admitted, where a deployment wants to name them. Empty admits
	// whoever IAP itself admitted, which is the ordinary case: IAP has an access policy, and
	// repeating it here means maintaining it twice.
	AllowedEmails []string

	// AllowedHostedDomains are the Google Workspace domains admitted, read from the hd claim.
	// Empty admits any.
	//
	// A consumer domain has no hd claim at all, so naming a domain here also turns away personal
	// accounts.
	AllowedHostedDomains []string

	// JwkUrl is where the signing keys are published. A nil one is IAP's.
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

// WithAllowedEmails adds to what an earlier call set rather than replacing it.
func WithAllowedEmails(emails ...string) Option {
	return func(config *Config) {
		config.AllowedEmails = append(config.AllowedEmails, emails...)
	}
}

// WithAllowedHostedDomains adds to what an earlier call set rather than replacing it.
func WithAllowedHostedDomains(domains ...string) Option {
	return func(config *Config) {
		config.AllowedHostedDomains = append(config.AllowedHostedDomains, domains...)
	}
}

func WithJwkUrl(jwkUrl *url.URL) Option {
	return func(config *Config) {
		config.JwkUrl = jwkUrl
	}
}
