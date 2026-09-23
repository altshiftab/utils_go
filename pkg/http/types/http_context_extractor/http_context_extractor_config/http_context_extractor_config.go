package http_context_extractor_config

import "github.com/altshiftab/utils_go/pkg/schema"

type MaskedHeader struct {
	Url     *schema.Url
	Headers []string
}

type Config struct {
	ReplaceableMessages    []string
	MaskedUrlParams        []*schema.Url
	MaskedHeaders          []*MaskedHeader
	MaskedRequestBodyUrls  []*schema.Url
	MaskedResponseBodyUrls []*schema.Url
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{}
	for _, option := range options {
		if option != nil {
			option(config)
		}
	}

	return config
}

func WithReplaceableMessages(replaceableMessages ...string) Option {
	return func(config *Config) {
		config.ReplaceableMessages = replaceableMessages
	}
}

// WithMaskedUrlParams names, per URL pattern, the query parameters that are credentials rather
// than identifiers.
//
// A pattern's own Query names the parameters; its Domain, Path and domain parts say which URLs it
// applies to, and an empty one of those matches anything. The parameters are masked wherever a URL
// carrying them appears in an entry, not only in the URL of the request being logged: the Referer
// it was reached from, and every URL a browser's violation report names. One declaration of what
// is secret therefore covers the page, whatever that page goes on to request, and whatever is
// reported about it.
func WithMaskedUrlParams(urlPatterns ...*schema.Url) Option {
	return func(config *Config) {
		config.MaskedUrlParams = urlPatterns
	}
}

func WithMaskedHeaders(maskedHeaders ...*MaskedHeader) Option {
	return func(config *Config) {
		config.MaskedHeaders = maskedHeaders
	}
}

func WithMaskedRequestBodyUrls(urlPatterns ...*schema.Url) Option {
	return func(config *Config) {
		config.MaskedRequestBodyUrls = urlPatterns
	}
}

func WithMaskedResponseBodyUrls(urlPatterns ...*schema.Url) Option {
	return func(config *Config) {
		config.MaskedResponseBodyUrls = urlPatterns
	}
}
