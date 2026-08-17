package problem_detail_config

import "github.com/altshiftab/utils_go/pkg/uuid"

type Config struct {
	Type      string
	Instance  string
	Detail    string
	Extension map[string]any
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{
		Instance: uuid.NewString(),
	}
	for _, option := range options {
		option(config)
	}

	return config
}

func WithType(t string) Option {
	return func(config *Config) {
		config.Type = t
	}
}

func WithInstance(instance string) Option {
	return func(config *Config) {
		config.Instance = instance
	}
}

func WithDetail(detail string) Option {
	return func(config *Config) {
		config.Detail = detail
	}
}

func WithExtension(extension map[string]any) Option {
	return func(config *Config) {
		config.Extension = extension
	}
}
