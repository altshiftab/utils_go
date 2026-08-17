package get_values_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type MajorDimension string

const (
	MajorDimensionRows    MajorDimension = "ROWS"
	MajorDimensionColumns MajorDimension = "COLUMNS"
)

type Config struct {
	MajorDimension MajorDimension
	FetchOptions   []fetch_config.Option
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{}
	for _, option := range options {
		option(config)
	}

	return config
}

func WithMajorDimension(majorDimension MajorDimension) Option {
	return func(config *Config) {
		config.MajorDimension = majorDimension
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
