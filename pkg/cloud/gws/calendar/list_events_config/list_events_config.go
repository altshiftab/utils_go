package list_events_config

import (
	"time"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

// The orders a listing can be in.
const (
	// OrderByStartTime is by when each starts, which Calendar gives only with SingleEvents.
	OrderByStartTime = "startTime"
	OrderByUpdated   = "updated"
)

type Config struct {
	// TimeMin and TimeMax bound the events listed: those that end after TimeMin and start before TimeMax. Zero leaves
	// that side open.
	TimeMin time.Time
	TimeMax time.Time
	// Query is words looked for in an event's summary, description, location, and its attendees' names and addresses.
	Query string
	// SingleEvents lists each occurrence of a recurring event on its own, rather than the event it recurs from once.
	SingleEvents bool
	OrderBy      string
	// MaxResults is how many are asked for per page. Zero takes Calendar's own default, which is 250; the most it
	// serves is 2500.
	MaxResults int
	PageToken  string
	// TimeZone is the IANA zone the times answered are to be in. Empty takes the calendar's own.
	TimeZone     string
	ShowDeleted  bool
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

func WithTimeMin(timeMin time.Time) Option {
	return func(config *Config) {
		config.TimeMin = timeMin
	}
}

func WithTimeMax(timeMax time.Time) Option {
	return func(config *Config) {
		config.TimeMax = timeMax
	}
}

func WithQuery(query string) Option {
	return func(config *Config) {
		config.Query = query
	}
}

func WithSingleEvents(singleEvents bool) Option {
	return func(config *Config) {
		config.SingleEvents = singleEvents
	}
}

func WithOrderBy(orderBy string) Option {
	return func(config *Config) {
		config.OrderBy = orderBy
	}
}

func WithMaxResults(maxResults int) Option {
	return func(config *Config) {
		config.MaxResults = maxResults
	}
}

func WithPageToken(pageToken string) Option {
	return func(config *Config) {
		config.PageToken = pageToken
	}
}

func WithTimeZone(timeZone string) Option {
	return func(config *Config) {
		config.TimeZone = timeZone
	}
}

func WithShowDeleted(showDeleted bool) Option {
	return func(config *Config) {
		config.ShowDeleted = showDeleted
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
