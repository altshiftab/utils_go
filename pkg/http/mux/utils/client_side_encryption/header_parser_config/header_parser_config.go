package header_parser_config

import "github.com/altshiftab/utils_go/pkg/json/jose/jwe"

const (
	DefaultHeaderName        = "X-Client-Public-Jwk"
	DefaultKeyAlgorithm      = jwe.KeyAlgorithmEcdhEs
	DefaultContentEncryption = jwe.ContentEncryptionA256Gcm
	DefaultContentType       = "application/json"
)

type Config struct {
	HeaderName        string
	KeyAlgorithm      jwe.KeyAlgorithm
	ContentEncryption jwe.ContentEncryption
	ContentType       string
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{
		HeaderName:        DefaultHeaderName,
		KeyAlgorithm:      DefaultKeyAlgorithm,
		ContentEncryption: DefaultContentEncryption,
		ContentType:       DefaultContentType,
	}
	for _, option := range options {
		option(config)
	}

	return config
}

func WithHeaderName(headerName string) Option {
	return func(config *Config) {
		config.HeaderName = headerName
	}
}

func WithKeyAlgorithm(keyAlgorithm jwe.KeyAlgorithm) Option {
	return func(config *Config) {
		config.KeyAlgorithm = keyAlgorithm
	}
}

func WithContentEncryption(contentEncryption jwe.ContentEncryption) Option {
	return func(config *Config) {
		config.ContentEncryption = contentEncryption
	}
}

func WithContentType(contentType string) Option {
	return func(config *Config) {
		config.ContentType = contentType
	}
}
