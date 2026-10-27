package http_context_extractor_config

import "github.com/altshiftab/utils_go/pkg/schema"

type MaskedHeader struct {
	Url     *schema.Url
	Headers []string
}

// MaskedBodyFields names, for the requests or the responses whose URL matches Url, what of their
// bodies the log masks: the values of the members so named in a JSON body, wherever in it they
// are, or of the keys so named in a form.
//
// AllValues masks every value instead, keeping only the body's shape -- which members were sent,
// not what they said -- for a body whose fields a client chooses, where a name-by-name list can be
// stepped around by sending the value under another. Every member name or form key but those in
// KeptNames is masked as well: such a client can send a value as a name just as well.
//
// A body of another type, one that does not parse -- one cut at the body limit among them -- and
// one matched by an entry naming no field without AllValues, is masked whole.
type MaskedBodyFields struct {
	Url       *schema.Url
	Fields    []string
	AllValues bool
	KeptNames []string
}

type Config struct {
	ReplaceableMessages      []string
	MaskedUrlParams          []*schema.Url
	MaskedHeaders            []*MaskedHeader
	MaskedRequestBodyUrls    []*schema.Url
	MaskedResponseBodyUrls   []*schema.Url
	MaskedRequestBodyFields  []*MaskedBodyFields
	MaskedResponseBodyFields []*MaskedBodyFields
	BodyLimit                int
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

func WithMaskedRequestBodyFields(maskedBodyFields ...*MaskedBodyFields) Option {
	return func(config *Config) {
		config.MaskedRequestBodyFields = maskedBodyFields
	}
}

func WithMaskedResponseBodyFields(maskedBodyFields ...*MaskedBodyFields) Option {
	return func(config *Config) {
		config.MaskedResponseBodyFields = maskedBodyFields
	}
}

func WithMaskedResponseBodyUrls(urlPatterns ...*schema.Url) Option {
	return func(config *Config) {
		config.MaskedResponseBodyUrls = urlPatterns
	}
}

// WithBodyLimit sets how much of a request's or a response's body is rendered,
// in bytes; what is past it is left out and said to be. One that is not
// positive leaves the default.
func WithBodyLimit(bodyLimit int) Option {
	return func(config *Config) {
		config.BodyLimit = bodyLimit
	}
}
