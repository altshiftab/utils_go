package http_logger_config

import (
	"io"
	"log/slog"
	"os"

	gcpHttpContextExtractor "github.com/altshiftab/utils_go/pkg/cloud/gcp/types/http_context_extractor"
	motmedelMux "github.com/altshiftab/utils_go/pkg/http/mux"
	"github.com/altshiftab/utils_go/pkg/http/types/http_context_extractor"
	"github.com/altshiftab/utils_go/pkg/http/types/http_context_extractor/http_context_extractor_config"
	motmedelHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

const DefaultLogLevel = slog.LevelInfo

// DefaultWriter is where the entries go unless told otherwise. Standard output is where a process
// whose logs are collected for it is expected to write them.
var DefaultWriter io.Writer = os.Stdout

// DefaultReplaceableMessages are the messages this module logs where what is worth reading about
// the request is in the HTTP context rather than in the message. The extractor that reads that
// context replaces them with what it says; see http_context_extractor_config.WithReplaceableMessages.
var DefaultReplaceableMessages = []string{
	motmedelMux.ClientErrorMessage,
	motmedelMux.ServerErrorMessage,
	motmedelMux.ResponseServedMessage,
	motmedelHttpUtils.FetchPerformedMessage,
}

type Config struct {
	Writer   io.Writer
	LogLevel slog.Level
	// HttpContextExtractor adds what is known about the request being logged. Unless one is set, an
	// extractor that replaces DefaultReplaceableMessages is.
	HttpContextExtractor *http_context_extractor.Extractor
	// Gcp makes the logger write what Google Cloud Logging expects of it: the names it reads the
	// severity and the message under, the fields it reads a request under, and entries within the
	// size it accepts. It is off by default, the rest being of no use to anything else.
	Gcp bool
	// GcpHttpContextExtractor adds what Cloud Logging reads a request under. Unless one is set, and
	// where Gcp is, a default one is.
	GcpHttpContextExtractor *gcpHttpContextExtractor.Extractor
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{
		Writer:   DefaultWriter,
		LogLevel: DefaultLogLevel,
		HttpContextExtractor: http_context_extractor.New(
			http_context_extractor_config.WithReplaceableMessages(DefaultReplaceableMessages...),
		),
	}

	for _, option := range options {
		if option != nil {
			option(config)
		}
	}

	return config
}

func WithWriter(writer io.Writer) Option {
	return func(config *Config) {
		config.Writer = writer
	}
}

func WithLogLevel(logLevel slog.Level) Option {
	return func(config *Config) {
		config.LogLevel = logLevel
	}
}

func WithHttpContextExtractor(httpContextExtractor *http_context_extractor.Extractor) Option {
	return func(config *Config) {
		config.HttpContextExtractor = httpContextExtractor
	}
}

// WithGcp makes the logger write what Google Cloud Logging expects of it, which nothing else reads.
func WithGcp(gcp bool) Option {
	return func(config *Config) {
		config.Gcp = gcp
	}
}

// WithGcpHttpContextExtractor sets what adds the fields Cloud Logging reads a request under. It is
// used only where WithGcp is.
func WithGcpHttpContextExtractor(gcpHttpContextExtractor *gcpHttpContextExtractor.Extractor) Option {
	return func(config *Config) {
		config.GcpHttpContextExtractor = gcpHttpContextExtractor
	}
}
