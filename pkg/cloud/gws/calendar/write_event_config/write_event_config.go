package write_event_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

// Whom Calendar tells of an event made, changed or deleted.
const (
	SendUpdatesAll          = "all"
	SendUpdatesExternalOnly = "externalOnly"
	SendUpdatesNone         = "none"
)

// Config is the options of a call that makes, changes or deletes an event.
type Config struct {
	// SendUpdates is whom Calendar mails about it. Empty takes Calendar's own default, which mails no one -- the
	// guests of an event made so are on it without being told.
	SendUpdates string
	// ConferenceDataVersion is 1 for a call whose event asks for a conference to be made, or carries one; 0 has
	// Calendar ignore the event's conference data.
	ConferenceDataVersion int
	FetchOptions          []fetch_config.Option
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{}
	for _, option := range options {
		option(config)
	}

	return config
}

func WithSendUpdates(sendUpdates string) Option {
	return func(config *Config) {
		config.SendUpdates = sendUpdates
	}
}

func WithConferenceDataVersion(conferenceDataVersion int) Option {
	return func(config *Config) {
		config.ConferenceDataVersion = conferenceDataVersion
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
