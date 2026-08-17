package http_logger

import (
	"bytes"
	"encoding/json/v2"
	"log/slog"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/log/entry_size_guard"
	"github.com/altshiftab/utils_go/pkg/log/http_logger/http_logger_config"
)

func logRecord(t *testing.T, buffer *bytes.Buffer) map[string]any {
	t.Helper()

	var record map[string]any
	if err := json.Unmarshal(buffer.Bytes(), &record); err != nil {
		t.Fatalf("json unmarshal log record: %v (%q)", err, buffer.String())
	}

	return record
}

// TestNew verifies that the logger writes JSON as the standard library names it, nothing being
// asked of it beyond that.
func TestNew(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer

	logger := New(http_logger_config.WithWriter(&buffer))
	if logger == nil || logger.Logger == nil {
		t.Fatal("nil logger")
	}

	logger.Info("test message")

	record := logRecord(t, &buffer)

	if record["msg"] != "test message" {
		t.Errorf("msg: got %v in %v", record["msg"], record)
	}
	if record["level"] != "INFO" {
		t.Errorf("level: got %v in %v", record["level"], record)
	}

	// What Cloud Logging reads is not written unless it is asked for.
	for _, key := range []string{"severity", "message"} {
		if value, found := record[key]; found {
			t.Errorf("%s: got %v, want nothing outside gcp mode", key, value)
		}
	}
}

// TestNewWithGcp verifies that the severity and the message are written under the names Cloud
// Logging reads them under, which is what the mode is for.
func TestNewWithGcp(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer

	logger := New(http_logger_config.WithWriter(&buffer), http_logger_config.WithGcp(true))
	if logger == nil || logger.Logger == nil {
		t.Fatal("nil logger")
	}

	logger.Info("test message")

	record := logRecord(t, &buffer)

	if record["message"] != "test message" {
		t.Errorf("message: got %v in %v", record["message"], record)
	}
	if record["severity"] != "INFO" {
		t.Errorf("severity: got %v in %v", record["severity"], record)
	}
}

// TestNewWithGcpGuardsTheEntrySize verifies that an entry too large for Cloud Logging to accept is
// reduced in gcp mode, and left alone otherwise -- nothing else rejecting it.
func TestNewWithGcpGuardsTheEntrySize(t *testing.T) {
	t.Parallel()

	oversized := strings.Repeat("a", entry_size_guard.DefaultEntryLimit)

	testCases := []struct {
		name              string
		gcp               bool
		expectedTruncated bool
	}{
		{name: "gcp", gcp: true, expectedTruncated: true},
		{name: "not gcp"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var buffer bytes.Buffer

			logger := New(
				http_logger_config.WithWriter(&buffer),
				http_logger_config.WithGcp(testCase.gcp),
			)

			logger.Info("test message", slog.String("padding", oversized))

			truncated := strings.Contains(buffer.String(), entry_size_guard.TruncatedKey)
			if truncated != testCase.expectedTruncated {
				t.Errorf("truncated: got %t, want %t", truncated, testCase.expectedTruncated)
			}

			withinLimit := buffer.Len() <= entry_size_guard.DefaultEntryLimit
			if withinLimit != testCase.expectedTruncated {
				t.Errorf("within the limit: got %t (%d bytes)", withinLimit, buffer.Len())
			}
		})
	}
}

func TestNewLogLevelFilters(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer

	logger := New(
		http_logger_config.WithWriter(&buffer),
		http_logger_config.WithLogLevel(slog.LevelWarn),
	)

	logger.Info("filtered")
	if buffer.Len() != 0 {
		t.Errorf("the info record was written, though the level is warn: %q", buffer.String())
	}

	logger.Warn("emitted")
	if buffer.Len() == 0 {
		t.Error("the warn record was not written")
	}
}
