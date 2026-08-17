package context_logger

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"log/slog"
	"testing"

	altshiftLog "github.com/altshiftab/utils_go/pkg/log"
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

type ctxKeyType struct{}

var ctxKey ctxKeyType

func TestNewRunsExtractorWithContext(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{ReplaceAttr: dropTime})

	extractor := altshiftLog.ContextExtractorFunction(func(ctx context.Context, record *slog.Record) error {
		if v, ok := ctx.Value(ctxKey).(string); ok {
			record.Add("from_context", v)
		}
		return nil
	})

	logger := New(handler, extractor)
	if logger == nil {
		t.Fatal("New returned nil logger")
	}

	ctx := context.WithValue(context.Background(), ctxKey, "value")
	logger.InfoContext(ctx, "hello", "explicit", "attr")

	m := parseJSON(t, buf)
	if m["from_context"] != "value" {
		t.Fatalf("from_context = %v, want value", m["from_context"])
	}
	if m["explicit"] != "attr" {
		t.Fatalf("explicit = %v, want attr", m["explicit"])
	}
	if m[slog.MessageKey] != "hello" {
		t.Fatalf("msg = %v, want hello", m[slog.MessageKey])
	}
}

func TestNewWithNilExtractorDoesNotPanic(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{ReplaceAttr: dropTime})

	logger := New(handler, nil)
	logger.InfoContext(context.Background(), "hello")

	m := parseJSON(t, buf)
	if m[slog.MessageKey] != "hello" {
		t.Fatalf("msg = %v, want hello", m[slog.MessageKey])
	}
}

func TestNewMergesDuplicateGroups(t *testing.T) {
	t.Parallel()

	// Two extractors each contribute to the same "meta" group; the wrapped
	// tree handler must merge them into a single group.
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, &slog.HandlerOptions{ReplaceAttr: dropTime})

	first := altshiftLog.ContextExtractorFunction(func(_ context.Context, record *slog.Record) error {
		record.Add(slog.Group("meta", slog.String("a", "1")))
		return nil
	})
	second := altshiftLog.ContextExtractorFunction(func(_ context.Context, record *slog.Record) error {
		record.Add(slog.Group("meta", slog.String("b", "2")))
		return nil
	})

	logger := New(handler, first, second)
	logger.InfoContext(context.Background(), "hello")

	m := parseJSON(t, buf)
	meta, ok := m["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta is not a merged group: %#v", m["meta"])
	}
	if meta["a"] != "1" || meta["b"] != "2" {
		t.Fatalf("meta not merged: %#v", meta)
	}
}
