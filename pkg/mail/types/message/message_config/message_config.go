package message_config

import (
	"net/mail"

	"github.com/altshiftab/utils_go/pkg/mail/types/message/message_header"
)

type Config struct {
	Cc      []*mail.Address
	Bcc     []*mail.Address
	ReplyTo []*mail.Address
	Domain  string
	Headers []*message_header.Header
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

func WithCc(cc []*mail.Address) Option {
	return func(config *Config) {
		config.Cc = cc
	}
}

func WithBcc(bcc []*mail.Address) Option {
	return func(config *Config) {
		config.Bcc = bcc
	}
}

func WithReplyTo(replyTo []*mail.Address) Option {
	return func(config *Config) {
		config.ReplyTo = replyTo
	}
}

func WithDomain(domain string) Option {
	return func(config *Config) {
		config.Domain = domain
	}
}

func WithHeaders(headers ...*message_header.Header) Option {
	return func(config *Config) {
		config.Headers = headers
	}
}
