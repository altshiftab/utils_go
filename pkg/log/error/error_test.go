package error_test

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"testing"

	context2 "github.com/altshiftab/utils_go/pkg/context"
	logerror "github.com/altshiftab/utils_go/pkg/log/error"
)

var (
	errBoom  = errors.New("boom")
	errCause = errors.New("cause")
)

// captureHandler records the last handled record and the error carried on the
// context (extracted immediately so no context.Context is retained).
type captureHandler struct {
	capturedErr error
	record      slog.Record
	called      bool
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(ctx context.Context, record slog.Record) error {
	if e, ok := ctx.Value(context2.ErrorContextKey).(error); ok {
		h.capturedErr = e
	}
	h.record = record.Clone()
	h.called = true
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func recordAttrs(record slog.Record) map[string]any {
	m := make(map[string]any)
	record.Attrs(func(attr slog.Attr) bool {
		m[attr.Key] = attr.Value.Any()
		return true
	})
	return m
}

func TestLogLevels(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		logFn     func(message string, err error, logger *slog.Logger, args ...any)
		wantLevel slog.Level
	}{
		{name: "error", logFn: logerror.LogError, wantLevel: slog.LevelError},
		{name: "warning", logFn: logerror.LogWarning, wantLevel: slog.LevelWarn},
		{name: "debug", logFn: logerror.LogDebug, wantLevel: slog.LevelDebug},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			handler := &captureHandler{}
			logger := slog.New(handler)

			testCase.logFn("a message", errBoom, logger, "key", "value")

			if !handler.called {
				t.Fatal("handler was not called")
			}
			if handler.record.Message != "a message" {
				t.Fatalf("message = %q, want %q", handler.record.Message, "a message")
			}
			if handler.record.Level != testCase.wantLevel {
				t.Fatalf("level = %v, want %v", handler.record.Level, testCase.wantLevel)
			}
			if !errors.Is(handler.capturedErr, errBoom) {
				t.Fatalf("context error = %v, want %v", handler.capturedErr, errBoom)
			}
			if attrs := recordAttrs(handler.record); attrs["key"] != "value" {
				t.Fatalf("record attrs = %#v, want key=value", attrs)
			}
		})
	}
}

//nolint:paralleltest // Mutates the global slog default logger; must not run in parallel.
func TestLogErrorNilLoggerUsesDefault(t *testing.T) {
	original := slog.Default()
	t.Cleanup(func() { slog.SetDefault(original) })

	handler := &captureHandler{}
	slog.SetDefault(slog.New(handler))

	logerror.LogError("default message", errBoom, nil)

	if !handler.called {
		t.Fatal("default handler was not called")
	}
	if handler.record.Message != "default message" {
		t.Fatalf("message = %q", handler.record.Message)
	}
	if !errors.Is(handler.capturedErr, errBoom) {
		t.Fatalf("context error = %v, want %v", handler.capturedErr, errBoom)
	}
}

// runFatalChild is executed in the forked subprocess for the fatal tests. It
// always exits the process.
func runFatalChild(mode string) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	switch mode {
	case "fatal":
		logerror.LogFatal("boom", errCause, logger)
	case "fatal_code":
		logerror.LogFatalWithExitCode("boom", errCause, logger, 3)
	case "fatal_exiting":
		logerror.LogFatalWithExitingMessage("boom", errCause, logger)
	}

	os.Exit(0)
}

func TestMain(m *testing.M) {
	if mode := os.Getenv("LOG_FATAL_MODE"); mode != "" {
		runFatalChild(mode)
	}
	os.Exit(m.Run())
}

func TestLogFatalExits(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		mode     string
		wantExit int
		wantMsg  string
	}{
		{name: "fatal", mode: "fatal", wantExit: 1, wantMsg: "boom"},
		{name: "fatal with code", mode: "fatal_code", wantExit: 3, wantMsg: "boom"},
		{name: "fatal exiting message", mode: "fatal_exiting", wantExit: 1, wantMsg: "boom Exiting."},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			//nolint:gosec // Re-executes this test binary with a constant argument.
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestLogFatalExits$")
			cmd.Env = append(os.Environ(), "LOG_FATAL_MODE="+testCase.mode)

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
