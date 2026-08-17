package create_permission_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	// SendNotificationEmail distinguishes unset (API default: email sent for
	// user and group grants) from an explicit false.
	SendNotificationEmail *bool
	EmailMessage          string
	TransferOwnership     bool
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

func WithSendNotificationEmail(sendNotificationEmail bool) Option {
	return func(config *Config) {
		config.SendNotificationEmail = &sendNotificationEmail
	}
}

// WithEmailMessage sets a custom message to include in the notification email.
func WithEmailMessage(emailMessage string) Option {
	return func(config *Config) {
		config.EmailMessage = emailMessage
	}
}

func WithTransferOwnership(transferOwnership bool) Option {
	return func(config *Config) {
		config.TransferOwnership = transferOwnership
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
