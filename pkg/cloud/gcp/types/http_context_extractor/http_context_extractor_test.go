package http_context_extractor

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/http_context_extractor/http_context_extractor_config"
	altshiftHttpContext "github.com/altshiftab/utils_go/pkg/http/context"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
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
		altshiftHttpContext.HttpContextContextKey,
		(*altshiftHttpTypes.HttpContext)(nil),
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

	httpContext := &altshiftHttpTypes.HttpContext{Request: request, Response: response}
	ctx := altshiftHttpContext.WithHttpContextValue(context.Background(), httpContext)

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

	httpContext := &altshiftHttpTypes.HttpContext{Request: request, Response: response}
	ctx := altshiftHttpContext.WithHttpContextValue(context.Background(), httpContext)

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

// Cloud Logging reads the referrer from a field this extractor builds itself, so it is the one
// place a service's URL masking has to be handed in rather than inherited.
func TestNewWithMaskUrl(t *testing.T) {
	t.Parallel()

	const referer = "https://example.com/verification?token=SECRET"

	extractor := New(http_context_extractor_config.WithMaskUrl(func(string) string {
		return "https://example.com/verification?token=(MASKED)"
	}))
	if extractor.MaskUrl == nil {
		t.Fatal("WithMaskUrl did not reach the extractor")
	}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/order/consented", nil)
	request.Header.Set("Referer", referer)

	record := &slog.Record{}
	ctx := altshiftHttpContext.WithHttpContextValue(
		context.Background(),
		&altshiftHttpTypes.HttpContext{Request: request},
	)

	if err := extractor.Handle(ctx, record); err != nil {
		t.Fatalf("handle: %v", err)
	}

	var found, masked bool
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key != "httpRequest" {
			return true
		}
		for _, field := range attr.Value.Resolve().Group() {
			if field.Key == "referer" {
				found = true
				masked = field.Value.String() != referer
				return false
			}
		}
		return true
	})

	if !found {
		t.Fatal("no httpRequest.referer in the record")
	}
	if !masked {
		t.Error("httpRequest.referer was written unmasked")
	}
}

func TestHandleInvalidUtf8(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		header string
	}{
		{name: "a user agent", header: "User-Agent"},
		{name: "a referrer", header: "Referer"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/foo", nil)
			request.Header[testCase.header] = []string{"caf\xe9"}
			ctx := altshiftHttpContext.WithHttpContextValue(
				context.Background(),
				&altshiftHttpTypes.HttpContext{Request: request, Response: &http.Response{StatusCode: http.StatusOK}},
			)

			record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
			if err := (&Extractor{ProjectId: "test-project"}).Handle(ctx, &record); err != nil {
				t.Fatalf("Handle: unexpected error: %v", err)
			}
			if _, ok := recordAttrs(&record)["httpRequest"]; !ok {
				t.Error("expected the httpRequest group")
			}
		})
	}
}

// httpRequestField returns a field of the httpRequest group the record carries, and whether there was
// one.
func httpRequestField(record *slog.Record, key string) (string, bool) {
	var value string
	var found bool
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key != "httpRequest" {
			return true
		}
		for _, field := range attr.Value.Resolve().Group() {
			if field.Key == key {
				value, found = field.Value.String(), true
				return false
			}
		}
		return true
	})

	return value, found
}

// Cloud Logging's remoteIp is the client the mux resolved, where it resolved one, and never the
// request's own forwarding headers.
func TestHandleRemoteIp(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		clientAddress string
		expected      string
	}{
		{name: "the resolved client", clientAddress: "192.0.2.1", expected: "192.0.2.1"},
		{name: "nothing resolved", clientAddress: "", expected: "203.0.113.9:54321"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com/", nil)
			request.RemoteAddr = "203.0.113.9:54321"
			request.Header.Set("X-Forwarded-For", "192.0.2.66")
			request.Header.Set("X-Client-Ip-Address", "192.0.2.67")

			ctx := altshiftHttpContext.WithHttpContextValue(
				context.Background(),
				&altshiftHttpTypes.HttpContext{Request: request, ClientAddress: testCase.clientAddress},
			)

			record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
			if err := (&Extractor{}).Handle(ctx, &record); err != nil {
				t.Fatalf("handle: %v", err)
			}

			remoteIp, found := httpRequestField(&record, "remoteIp")
			if !found {
				t.Fatal("no httpRequest.remoteIp in the record")
			}
			if remoteIp != testCase.expected {
				t.Errorf("httpRequest.remoteIp = %q, want %q", remoteIp, testCase.expected)
			}
		})
	}
}
