package http_context_extractor

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/http_context_extractor/http_context_extractor_config"
	motmedelHttpContext "github.com/altshiftab/utils_go/pkg/http/context"
	motmedelHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

func TestNew(t *testing.T) {
	t.Parallel()

	if extractor := New(); extractor.ProjectId != "" {
		t.Errorf("default ProjectId = %q, want empty", extractor.ProjectId)
	}

	extractor := New(http_context_extractor_config.WithProjectId("my-project"))
	if extractor.ProjectId != "my-project" {
		t.Errorf("ProjectId = %q, want %q", extractor.ProjectId, "my-project")
	}
}

func recordAttrs(record *slog.Record) map[string]slog.Value {
	attrs := make(map[string]slog.Value)
	record.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value
		return true
	})
	return attrs
}

func TestHandleNilRecord(t *testing.T) {
	t.Parallel()

	extractor := &Extractor{ProjectId: "p"}
	if err := extractor.Handle(context.Background(), nil); err != nil {
		t.Errorf("Handle(nil record) = %v, want nil", err)
	}
}

func TestHandleNoHttpContext(t *testing.T) {
	t.Parallel()

	extractor := &Extractor{ProjectId: "p"}
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	if err := extractor.Handle(context.Background(), &record); err != nil {
		t.Fatalf("Handle: unexpected error: %v", err)
	}
	if record.NumAttrs() != 0 {
		t.Errorf("expected no attrs added, got %d", record.NumAttrs())
	}
}

func TestHandleNilHttpContextValue(t *testing.T) {
	t.Parallel()

	extractor := &Extractor{ProjectId: "p"}
	ctx := context.WithValue(
		context.Background(),
		motmedelHttpContext.HttpContextContextKey,
		(*motmedelHttpTypes.HttpContext)(nil),
	)
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	if err := extractor.Handle(ctx, &record); err != nil {
		t.Fatalf("Handle: unexpected error: %v", err)
	}
	if record.NumAttrs() != 0 {
		t.Errorf("expected no attrs added, got %d", record.NumAttrs())
	}
}

func TestHandleWithHttpContext(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/foo", nil)
	request.Header.Set("X-Cloud-Trace-Context", "105445aa7843bc8bf206b120001000/123;o=1")
	response := &http.Response{StatusCode: http.StatusOK}

	httpContext := &motmedelHttpTypes.HttpContext{Request: request, Response: response}
	ctx := motmedelHttpContext.WithHttpContextValue(context.Background(), httpContext)

	extractor := &Extractor{ProjectId: "test-project"}
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	if err := extractor.Handle(ctx, &record); err != nil {
		t.Fatalf("Handle: unexpected error: %v", err)
	}

	if record.NumAttrs() == 0 {
		t.Fatal("expected attributes to be added, got none")
	}

	attrs := recordAttrs(&record)

	traceValue, ok := attrs["logging.googleapis.com/trace"]
	if !ok {
		t.Fatalf("expected trace attribute, got attrs %#v", attrs)
	}
	wantTrace := "projects/test-project/traces/105445aa7843bc8bf206b120001000"
	if traceValue.String() != wantTrace {
		t.Errorf("trace = %q, want %q", traceValue.String(), wantTrace)
	}

	if _, ok := attrs["httpRequest"]; !ok {
		t.Errorf("expected httpRequest group attribute, got attrs %#v", attrs)
	}
}

func TestHandleWithHttpContextNoProjectId(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/foo", nil)
	request.Header.Set("X-Cloud-Trace-Context", "105445aa7843bc8bf206b120001000/123;o=1")
	response := &http.Response{StatusCode: http.StatusOK}

	httpContext := &motmedelHttpTypes.HttpContext{Request: request, Response: response}
	ctx := motmedelHttpContext.WithHttpContextValue(context.Background(), httpContext)

	// With an empty ProjectId, no formatted "trace" attribute should be produced.
	extractor := &Extractor{}
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	if err := extractor.Handle(ctx, &record); err != nil {
		t.Fatalf("Handle: unexpected error: %v", err)
	}

	attrs := recordAttrs(&record)
	if _, ok := attrs["logging.googleapis.com/trace"]; ok {
		t.Errorf("did not expect trace attribute without a project id, got attrs %#v", attrs)
	}
}
