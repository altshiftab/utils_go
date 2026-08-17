package list_history_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type HistoryType string

const (
	HistoryTypeMessageAdded   HistoryType = "messageAdded"
	HistoryTypeMessageDeleted HistoryType = "messageDeleted"
	HistoryTypeLabelAdded     HistoryType = "labelAdded"
	HistoryTypeLabelRemoved   HistoryType = "labelRemoved"
)

type Config struct {
	HistoryTypes []HistoryType
	LabelId      string
	FetchOptions []fetch_config.Option
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{}
	for _, option := range options {
		option(config)
	}

	return config
}

func WithHistoryTypes(historyTypes ...HistoryType) Option {
	return func(config *Config) {
		config.HistoryTypes = append(config.HistoryTypes, historyTypes...)
	}
}

func WithLabelId(labelId string) Option {
	return func(config *Config) {
		config.LabelId = labelId
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
