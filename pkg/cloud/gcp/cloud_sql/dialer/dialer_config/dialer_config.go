package dialer_config

import (
	"net"
	"time"
)

const (
	IpTypePublic  = "PUBLIC"
	IpTypePrivate = "PRIVATE"

	DefaultPort = "3307"
)

type Config struct {
	IpType    string
	Port      string
	NetDialer *net.Dialer
	// RefreshBuffer is how long before its expiry a client certificate is replaced.
	RefreshBuffer time.Duration
	Now           func() time.Time
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{
		IpType:        IpTypePublic,
		Port:          DefaultPort,
		NetDialer:     &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second},
		RefreshBuffer: 4 * time.Minute,
		Now:           time.Now,
	}
	for _, option := range options {
		option(config)
	}

	return config
}

func WithIpType(ipType string) Option {
	return func(config *Config) {
		config.IpType = ipType
	}
}

// WithPort replaces the server-side proxy port, for tests.
func WithPort(port string) Option {
	return func(config *Config) {
		config.Port = port
	}
}

func WithNetDialer(netDialer *net.Dialer) Option {
	return func(config *Config) {
		config.NetDialer = netDialer
	}
}

func WithRefreshBuffer(refreshBuffer time.Duration) Option {
	return func(config *Config) {
		config.RefreshBuffer = refreshBuffer
	}
}

func WithNow(now func() time.Time) Option {
	return func(config *Config) {
		config.Now = now
	}
}
