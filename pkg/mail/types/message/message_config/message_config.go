package message_config

import (
	"net/mail"
	"time"

	"github.com/altshiftab/utils_go/pkg/mail/types/message/message_header"
)

type Config struct {
	Cc      []*mail.Address
	Bcc     []*mail.Address
	ReplyTo []*mail.Address
	Domain  string
	Headers []*message_header.Header
	// Date is when the message is dated, in its own zone; the time it is written, in the process's, if zero.
	Date time.Time
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

// WithDate dates the message, the Date header written in the time's own zone.
func WithDate(date time.Time) Option {
	return func(config *Config) {
		config.Date = date
	}
}

func WithHeaders(headers ...*message_header.Header) Option {
	return func(config *Config) {
		config.Headers = headers
	}
}
