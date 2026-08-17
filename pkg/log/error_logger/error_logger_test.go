package error_logger

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"testing"

	altshiftLog "github.com/altshiftab/utils_go/pkg/log"
)

var (
	errOrig  = errors.New("orig")
	errCause = errors.New("cause")
)

func dropTime(groups []string, attr slog.Attr) slog.Attr {
	if len(groups) == 0 && attr.Key == slog.TimeKey {
		return slog.Attr{}
	}
	return attr
}

func parseJSON(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
		t.Fatalf("failed to parse %q: %v", buf.String(), err)
	}
	return m
}

func dig(t *testing.T, m map[string]any, keys ...string) any {
	t.Helper()
	var current any = m
	for _, key := range keys {
		asMap, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("expected map at key %q, got %T", key, current)
		}
		current, ok = asMap[key]
		if !ok {
			t.Fatalf("missing key %q in %#v", key, asMap)
		}
	}
	return current
}

func TestErrorLogsErrorWithInput(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	logger := New(slog.NewJSONHandler(buf, &slog.HandlerOptions{ReplaceAttr: dropTime}))

	logger.Error("a message", errOrig, "the-input")

	m := parseJSON(t, buf)
	if m[slog.MessageKey] != "a message" {
		t.Fatalf("msg = %v, want 'a message'", m[slog.MessageKey])
	}
	if m[slog.LevelKey] != "ERROR" {
		t.Fatalf("level = %v, want ERROR", m[slog.LevelKey])
	}
	if got := dig(t, m, "error", "message"); got != "orig" {
		t.Fatalf("error.message = %v, want orig", got)
	}
	if got := dig(t, m, "error", "input", "value"); got != "the-input" {
		t.Fatalf("error.input.value = %v, want the-input", got)
	}
}

func TestWarningLogsAtWarnLevel(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	logger := New(slog.NewJSONHandler(buf, &slog.HandlerOptions{ReplaceAttr: dropTime}))

	logger.Warning("warn message", errOrig)

	m := parseJSON(t, buf)
	if m[slog.LevelKey] != "WARN" {
		t.Fatalf("level = %v, want WARN", m[slog.LevelKey])
	}
	if got := dig(t, m, "error", "message"); got != "orig" {
		t.Fatalf("error.message = %v, want orig", got)
	}
}

func TestErrorWithSkippingMessage(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	logger := New(slog.NewJSONHandler(buf, &slog.HandlerOptions{ReplaceAttr: dropTime}))

	logger.ErrorWithSkippingMessage("op failed", errOrig)

	m := parseJSON(t, buf)
	if m[slog.MessageKey] != "op failed Skipping." {
		t.Fatalf("msg = %v, want 'op failed Skipping.'", m[slog.MessageKey])
	}
}

func TestNewWithErrorContextExtractorSkipInput(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	logger := NewWithErrorContextExtractor(
		slog.NewJSONHandler(buf, &slog.HandlerOptions{ReplaceAttr: dropTime}),
		&altshiftLog.ErrorContextExtractor{SkipInput: true},
	)

	logger.Error("a message", errOrig, "the-input")

	m := parseJSON(t, buf)
	errorGroup, ok := m["error"].(map[string]any)
	if !ok {
		t.Fatalf("error group missing: %#v", m)
	}
	if _, ok := errorGroup["input"]; ok {
		t.Fatalf("input should be skipped: %#v", errorGroup)
	}
	if errorGroup["message"] != "orig" {
		t.Fatalf("error.message = %v, want orig", errorGroup["message"])
	}
}

func runFatalChild(mode string) {
	logger := New(slog.NewJSONHandler(os.Stdout, nil))

	switch mode {
	case "fatal":
		logger.Fatal("boom", errCause)
	case "fatal_exiting":
		logger.FatalWithExitingMessage("boom", errCause)
	}

	os.Exit(0)
}

func TestMain(m *testing.M) {
	if mode := os.Getenv("ERROR_LOGGER_FATAL_MODE"); mode != "" {
		runFatalChild(mode)
	}
	os.Exit(m.Run())
}

func TestFatalExits(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		mode     string
		wantExit int
		wantMsg  string
	}{
		{name: "fatal", mode: "fatal", wantExit: 1, wantMsg: "boom"},
		{name: "fatal exiting message", mode: "fatal_exiting", wantExit: 1, wantMsg: "boom Exiting."},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			//nolint:gosec // Re-executes this test binary with a constant argument.
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestFatalExits$")
			cmd.Env = append(os.Environ(), "ERROR_LOGGER_FATAL_MODE="+testCase.mode)

			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out

			runErr := cmd.Run()

			if got := exitCode(runErr); got != testCase.wantExit {
				t.Fatalf("exit code = %d, want %d (output: %q)", got, testCase.wantExit, out.String())
			}
			if got := extractMsg(t, out.Bytes()); got != testCase.wantMsg {
				t.Fatalf("logged msg = %q, want %q", got, testCase.wantMsg)
			}
		})
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return exitErr.ExitCode()
	}
	return -1
}

func extractMsg(t *testing.T, out []byte) string {
	t.Helper()
	for line := range bytes.SplitSeq(out, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var m map[string]any
		if json.Unmarshal(line, &m) != nil {
			continue
		}
		if msg, ok := m[slog.MessageKey].(string); ok {
			return msg
		}
	}
	t.Fatalf("no JSON log line found in output: %q", out)
	return ""
}
