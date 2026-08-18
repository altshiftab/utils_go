package journal_handler

import (
	"context"
	"encoding/json/v2"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/log/journal"
)

var errSendFailed = errors.New("journal unavailable")

type sentEntry struct {
	message  string
	priority journal.Priority
}

// newRecordingHandler returns a handler whose entries are collected rather than
// written to the journal, so the tests need no journal at all.
func newRecordingHandler(handlerOptions *slog.HandlerOptions) (*Handler, func() []sentEntry) {
	var mutex sync.Mutex
	var entries []sentEntry

	handler := NewJsonHandler(handlerOptions)
	handler.send = func(message string, priority journal.Priority, _ map[string]string) error {
		mutex.Lock()
		defer mutex.Unlock()
		entries = append(entries, sentEntry{message: message, priority: priority})
		return nil
	}

	return handler, func() []sentEntry {
		mutex.Lock()
		defer mutex.Unlock()
		return append([]sentEntry(nil), entries...)
	}
}

func TestLevelBecomesPriority(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		level    slog.Level
		expected journal.Priority
	}{
		{name: "debug", level: slog.LevelDebug, expected: journal.PriorityDebug},
		{name: "info", level: slog.LevelInfo, expected: journal.PriorityInfo},
		{name: "warn", level: slog.LevelWarn, expected: journal.PriorityWarning},
		{name: "error", level: slog.LevelError, expected: journal.PriorityError},
		{name: "an unmapped level falls back to info", level: slog.Level(42), expected: journal.PriorityInfo},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			handler, collected := newRecordingHandler(&slog.HandlerOptions{Level: slog.LevelDebug})

			logger := slog.New(handler)
			logger.Log(context.Background(), testCase.level, "a message")

			entries := collected()
			if len(entries) != 1 {
				t.Fatalf("expected one entry, got %d", len(entries))
			}

			if entries[0].priority != testCase.expected {
				t.Errorf("expected priority %d, got %d", testCase.expected, entries[0].priority)
			}
		})
	}
}

func TestRecordIsSentAsJson(t *testing.T) {
	t.Parallel()

	handler, collected := newRecordingHandler(nil)

	slog.New(handler).Info("a message", slog.String("key", "value"))

	entries := collected()
	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(entries))
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(entries[0].message), &decoded); err != nil {
		t.Fatalf("expected the entry to be JSON, got %q (%v)", entries[0].message, err)
	}

	if decoded["msg"] != "a message" {
		t.Errorf("expected the message, got %v", decoded["msg"])
	}

	if decoded["key"] != "value" {
		t.Errorf("expected the attribute, got %v", decoded["key"])
	}
}

func TestWithAttrsAndWithGroupKeepTheSendPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		build func(logger *slog.Logger) *slog.Logger
		check func(t *testing.T, decoded map[string]any)
	}{
		{
			name:  "with attrs",
			build: func(logger *slog.Logger) *slog.Logger { return logger.With(slog.String("carried", "yes")) },
			check: func(t *testing.T, decoded map[string]any) {
				if decoded["carried"] != "yes" {
					t.Errorf("expected the carried attribute, got %v", decoded["carried"])
				}
			},
		},
		{
			name:  "with group",
			build: func(logger *slog.Logger) *slog.Logger { return logger.WithGroup("grouped") },
			check: func(t *testing.T, decoded map[string]any) {
				group, ok := decoded["grouped"].(map[string]any)
				if !ok {
					t.Fatalf("expected a group, got %v", decoded["grouped"])
				}
				if group["key"] != "value" {
					t.Errorf("expected the grouped attribute, got %v", group["key"])
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			handler, collected := newRecordingHandler(nil)

			testCase.build(slog.New(handler)).Info("a message", slog.String("key", "value"))

			entries := collected()
			if len(entries) != 1 {
				t.Fatalf("expected one entry, got %d", len(entries))
			}

			var decoded map[string]any
			if err := json.Unmarshal([]byte(entries[0].message), &decoded); err != nil {
				t.Fatalf("expected JSON, got %q (%v)", entries[0].message, err)
			}

			testCase.check(t, decoded)
		})
	}
}

func TestEnabledFollowsTheLevel(t *testing.T) {
	t.Parallel()

	handler, _ := newRecordingHandler(&slog.HandlerOptions{Level: slog.LevelWarn})

	testCases := []struct {
		name     string
		level    slog.Level
		expected bool
	}{
		{name: "below the threshold", level: slog.LevelInfo, expected: false},
		{name: "at the threshold", level: slog.LevelWarn, expected: true},
		{name: "above the threshold", level: slog.LevelError, expected: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := handler.Enabled(context.Background(), testCase.level); got != testCase.expected {
				t.Errorf("expected %v, got %v", testCase.expected, got)
			}
		})
	}
}

func TestSendFailureIsReported(t *testing.T) {
	t.Parallel()

	handler := NewJsonHandler(nil)
	handler.send = func(string, journal.Priority, map[string]string) error {
		return errSendFailed
	}

	err := handler.Handle(context.Background(), slog.NewRecord(time.Time{}, slog.LevelInfo, "a message", 0))
	if err == nil {
		t.Fatal("expected an error")
	}

	if !errors.Is(err, errSendFailed) {
		t.Errorf("expected the send error to be wrapped, got %v", err)
	}
}

func TestConcurrentRecordsKeepTheirOwnPriority(t *testing.T) {
	t.Parallel()

	handler, collected := newRecordingHandler(&slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)

	levels := []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError}

	var waitGroup sync.WaitGroup
	for index := range 64 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			logger.Log(context.Background(), levels[index%len(levels)], "a message")
		}()
	}
	waitGroup.Wait()

	entries := collected()
	if len(entries) != 64 {
		t.Fatalf("expected 64 entries, got %d", len(entries))
	}

	// Every entry must carry a priority from the mapped set: a torn priority
	// would show up as a value none of the levels map to.
	valid := map[journal.Priority]bool{
		journal.PriorityDebug:   true,
		journal.PriorityInfo:    true,
		journal.PriorityWarning: true,
		journal.PriorityError:   true,
	}
	for _, entry := range entries {
		if !valid[entry.priority] {
			t.Errorf("unexpected priority %d", entry.priority)
		}
	}
}
