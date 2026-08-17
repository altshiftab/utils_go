package http_logger_config

import (
	"bytes"
	"log/slog"
	"testing"

	gcpHttpContextExtractor "github.com/altshiftab/utils_go/pkg/cloud/gcp/types/http_context_extractor"
	"github.com/altshiftab/utils_go/pkg/http/types/http_context_extractor"
)

func TestNew(t *testing.T) {
	t.Parallel()

	config := New(nil)
	if config == nil {
		t.Fatal("nil config")
	}

	if config.Writer != DefaultWriter {
		t.Errorf("writer: got %v", config.Writer)
	}
	if config.LogLevel != DefaultLogLevel {
		t.Errorf("log level: got %v", config.LogLevel)
	}

	// What Cloud Logging wants is of no use to anything else, so it is asked for rather than given.
	if config.Gcp {
		t.Error("gcp")
	}
	if config.GcpHttpContextExtractor != nil {
		t.Errorf("gcp http context extractor: got %v, want none", config.GcpHttpContextExtractor)
	}

	httpContextExtractor := config.HttpContextExtractor
	if httpContextExtractor == nil {
		t.Fatal("nil http context extractor")
	}

	// The messages this module logs in place of what the HTTP context says are replaced by default.
	if len(httpContextExtractor.ReplaceableMessages) != len(DefaultReplaceableMessages) {
		t.Fatalf(
			"replaceable messages: got %v, want %v",
			httpContextExtractor.ReplaceableMessages,
			DefaultReplaceableMessages,
		)
	}
	for _, message := range DefaultReplaceableMessages {
		if _, found := httpContextExtractor.ReplaceableMessages[message]; !found {
			t.Errorf("replaceable messages lack %q: %v", message, httpContextExtractor.ReplaceableMessages)
		}
	}
}

func TestOptions(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	httpContextExtractor := http_context_extractor.New()
	gcpExtractor := gcpHttpContextExtractor.New()

	config := New(
		WithWriter(&buffer),
		WithLogLevel(slog.LevelError),
		WithHttpContextExtractor(httpContextExtractor),
		WithGcp(true),
		WithGcpHttpContextExtractor(gcpExtractor),
	)
	if config == nil {
		t.Fatal("nil config")
	}

	if config.Writer != &buffer {
		t.Errorf("writer: got %v", config.Writer)
	}
	if config.LogLevel != slog.LevelError {
		t.Errorf("log level: got %v, want %v", config.LogLevel, slog.LevelError)
	}
	if config.HttpContextExtractor != httpContextExtractor {
		t.Error("the http context extractor is not the configured one")
	}
	if !config.Gcp {
		t.Error("no gcp")
	}
	if config.GcpHttpContextExtractor != gcpExtractor {
		t.Error("the gcp http context extractor is not the configured one")
	}
}
