package cloud_sql

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/types/export_context"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/types/operation"
)

// fakeAdmin models an instance's operations as the Admin API runs them: one at a time, each
// passing through PENDING and RUNNING before it is DONE, and refused with a 409 while another is
// under way.
type fakeAdmin struct {
	t *testing.T

	mu sync.Mutex
	// polls counts the gets of the running operation; it is done after donePolls.
	polls     int
	donePolls int
	running   *operation.Operation
	failure   *operation.OperationErrors
	lastBody  map[string]any
}

func (f *fakeAdmin) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/sql/v1beta4/projects/proj/instances/db/export":
		if f.running != nil && f.running.Status != operation.StatusDone {
			w.WriteHeader(http.StatusConflict)
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			f.t.Errorf("io read all: %v", err)
			return
		}
		f.lastBody = map[string]any{}
		if err := json.Unmarshal(raw, &f.lastBody); err != nil {
			f.t.Errorf("json unmarshal: %v", err)
		}
		f.polls = 0
		f.running = &operation.Operation{Name: "op-1", Status: operation.StatusPending, OperationType: operation.TypeExport}
		_ = json.MarshalWrite(w, f.running)
	case r.Method == http.MethodGet && r.URL.Path == "/sql/v1beta4/projects/proj/operations/op-1" && f.running != nil:
		f.polls++
		switch {
		case f.polls >= f.donePolls:
			f.running.Status = operation.StatusDone
			f.running.Error = f.failure
		case f.polls > 1:
			f.running.Status = operation.StatusRunning
		}
		_ = json.MarshalWrite(w, f.running)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestExportAndWait(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		databases     []string
		failure       *operation.OperationErrors
		wantFailed    bool
		wantDatabases bool
	}{
		{name: "the entire instance"},
		{name: "named databases", databases: []string{"signals_dd"}, wantDatabases: true},
		{
			name:       "a failed export",
			failure:    &operation.OperationErrors{Errors: []*operation.OperationError{{Code: "ERROR_RDBMS", Message: "pg_dump: error"}}},
			wantFailed: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			admin := &fakeAdmin{t: t, donePolls: 3, failure: testCase.failure}
			client := testClient(t, admin.handle)

			started, err := client.Export(t.Context(), "proj", "db", &export_context.ExportContext{
				Kind:             export_context.Kind,
				Uri:              "gs://staging/2026-11-19/proj/db",
				Databases:        testCase.databases,
				FileType:         export_context.FileTypeSql,
				Offload:          true,
				SqlExportOptions: &export_context.SqlExportOptions{Parallel: true, Threads: 2},
			})
			if err != nil {
				t.Fatalf("export: %v", err)
			}

			done, err := client.WaitForOperation(t.Context(), "proj", started.Name, time.Millisecond)
			if testCase.wantFailed {
				if !errors.Is(err, ErrOperationFailed) {
					t.Fatalf("wait for operation: %v, want %v", err, ErrOperationFailed)
				}
				if !strings.Contains(err.Error(), "pg_dump: error") {
					t.Errorf("the failure %q does not say why", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("wait for operation: %v", err)
			}
			if !done.Done() {
				t.Errorf("returned an operation %s", done.Status)
			}

			admin.mu.Lock()
			defer admin.mu.Unlock()
			if admin.polls != 3 {
				t.Errorf("polled %d times, want 3", admin.polls)
			}
			exportContext, ok := admin.lastBody["exportContext"].(map[string]any)
			if !ok {
				t.Fatalf("body %v has no export context", admin.lastBody)
			}
			// An entire-instance export is one that names no databases: the field must be
			// absent, not an empty list.
			if _, present := exportContext["databases"]; present != testCase.wantDatabases {
				t.Errorf("databases present %t, want %t", present, testCase.wantDatabases)
			}
			if exportContext["offload"] != true || exportContext["fileType"] != "SQL" || exportContext["uri"] != "gs://staging/2026-11-19/proj/db" {
				t.Errorf("export context %v", exportContext)
			}
			options, ok := exportContext["sqlExportOptions"].(map[string]any)
			if !ok || options["parallel"] != true || options["threads"] != float64(2) {
				t.Errorf("sql export options %v", options)
			}
		})
	}
}

func TestExportRefusedWhileRunning(t *testing.T) {
	t.Parallel()

	admin := &fakeAdmin{t: t, donePolls: 100}
	client := testClient(t, admin.handle)
	exportContext := &export_context.ExportContext{Uri: "gs://staging/x"}

	if _, err := client.Export(t.Context(), "proj", "db", exportContext); err != nil {
		t.Fatalf("export: %v", err)
	}
	if _, err := client.Export(t.Context(), "proj", "db", exportContext); err == nil {
		t.Error("a second export while the first runs: no error")
	}
}

func TestWaitForOperationStopsWithTheContext(t *testing.T) {
	t.Parallel()

	admin := &fakeAdmin{t: t, donePolls: 1 << 30}
	client := testClient(t, admin.handle)
	if _, err := client.Export(t.Context(), "proj", "db", &export_context.ExportContext{Uri: "gs://staging/x"}); err != nil {
		t.Fatalf("export: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()

	if _, err := client.WaitForOperation(ctx, "proj", "op-1", time.Millisecond); err == nil {
		t.Error("wait for an operation that never ends: no error")
	}
}

func TestExportRejects(t *testing.T) {
	t.Parallel()

	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("a request was made for an export refused before any")
		w.WriteHeader(http.StatusInternalServerError)
	})

	testCases := []struct {
		name          string
		project       string
		instance      string
		exportContext *export_context.ExportContext
	}{
		{name: "no project", instance: "db", exportContext: &export_context.ExportContext{Uri: "gs://b/x"}},
		{name: "no instance", project: "proj", exportContext: &export_context.ExportContext{Uri: "gs://b/x"}},
		{name: "no export context", project: "proj", instance: "db"},
		{name: "no uri", project: "proj", instance: "db", exportContext: &export_context.ExportContext{}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := client.Export(t.Context(), testCase.project, testCase.instance, testCase.exportContext); err == nil {
				t.Error("export: no error")
			}
		})
	}

	if _, err := client.WaitForOperation(t.Context(), "proj", "op", 0); err == nil {
		t.Error("wait for operation with no interval: no error")
	}
}
