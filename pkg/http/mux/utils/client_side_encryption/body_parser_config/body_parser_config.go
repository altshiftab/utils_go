package body_parser_config

import "github.com/altshiftab/utils_go/pkg/json/jose/jwe"

const (
	DefaultKeyAlgorithm      = jwe.KeyAlgorithmEcdhEs
	DefaultContentEncryption = jwe.ContentEncryptionA256Gcm
)

type Config struct {
	KeyIdentifier     string
	KeyAlgorithm      jwe.KeyAlgorithm
	ContentEncryption jwe.ContentEncryption
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{
		KeyAlgorithm:      DefaultKeyAlgorithm,
		ContentEncryption: DefaultContentEncryption,
	}
	for _, option := range options {
		option(config)
	}

	return config
}

func WithKeyIdentifier(keyIdentifier string) Option {
	return func(config *Config) {
		config.KeyIdentifier = keyIdentifier
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
