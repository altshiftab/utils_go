package import_message_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

// InternalDateSource is where Gmail takes an imported message's internal date
// from, which is what the mailbox sorts and dates it by.
type InternalDateSource string

const (
	// InternalDateSourceReceivedTime dates the message at the moment it is
	// imported. Gmail's default.
	InternalDateSourceReceivedTime InternalDateSource = "receivedTime"
	// InternalDateSourceDateHeader dates the message by its own Date header.
	InternalDateSourceDateHeader InternalDateSource = "dateHeader"
)

type Config struct {
	InternalDateSource InternalDateSource
	// NeverMarkSpam keeps Gmail's spam classifier from filing the message as
	// spam, whatever it makes of it.
	NeverMarkSpam bool
	// ProcessForCalendar has Gmail add the invitations the message carries to
	// the user's calendar.
	ProcessForCalendar bool
	// Deleted marks the message as permanently deleted, visible only to a
	// Workspace administrator.
	Deleted      bool
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

func WithInternalDateSource(source InternalDateSource) Option {
	return func(config *Config) {
		config.InternalDateSource = source
	}
}

func WithNeverMarkSpam(neverMarkSpam bool) Option {
	return func(config *Config) {
		config.NeverMarkSpam = neverMarkSpam
	}
}

func WithProcessForCalendar(processForCalendar bool) Option {
	return func(config *Config) {
		config.ProcessForCalendar = processForCalendar
	}
}

func WithDeleted(deleted bool) Option {
	return func(config *Config) {
		config.Deleted = deleted
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
