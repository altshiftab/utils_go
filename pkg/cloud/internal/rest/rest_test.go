package rest

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type testValue struct {
	Name string `json:"name"`
}

type testPage struct {
	Items         []string `json:"items"`
	NextPageToken string   `json:"nextPageToken"`
}

func testServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func TestGetJson(t *testing.T) {
	t.Parallel()

	server := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &testValue{Name: "value"}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	value, err := GetJson[testValue](context.Background(), server.URL, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value.Name != "value" {
		t.Errorf("expected name 'value', got %q", value.Name)
	}
}

func TestGetJson_CancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := GetJson[testValue](ctx, "http://localhost", nil); err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
}

func TestSendJson(t *testing.T) {
	t.Parallel()

	server := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}

		var input testValue
		if err := json.UnmarshalRead(r.Body, &input); err != nil {
			t.Errorf("decode: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &testValue{Name: input.Name + "-updated"}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	value, err := SendJson[testValue](
		context.Background(),
		http.MethodPut,
		server.URL,
		&testValue{Name: "value"},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value.Name != "value-updated" {
		t.Errorf("expected name 'value-updated', got %q", value.Name)
	}
}

func TestDo(t *testing.T) {
	t.Parallel()

	var seenMethod string
	server := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	})

	if err := Do(context.Background(), http.MethodDelete, server.URL, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seenMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", seenMethod)
	}
}

func TestDoWithBody(t *testing.T) {
	t.Parallel()

	var seenBody testValue
	server := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := json.UnmarshalRead(r.Body, &seenBody); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte("{}")); err != nil {
			t.Errorf("write: %v", err)
		}
	})

	err := DoWithBody(context.Background(), http.MethodPost, server.URL, &testValue{Name: "value"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seenBody.Name != "value" {
		t.Errorf("expected body name 'value', got %q", seenBody.Name)
	}
}

func TestListPaginated(t *testing.T) {
	t.Parallel()

	server := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		page := &testPage{Items: []string{"a", "b"}, NextPageToken: "next"}
		if r.URL.Query().Get("pageToken") == "next" {
			page = &testPage{Items: []string{"c"}}
		}

		if err := json.MarshalWrite(w, page); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	items, err := ListPaginated(
		context.Background(),
		func(pageToken string) string {
			if pageToken == "" {
				return server.URL
			}
			return server.URL + "?pageToken=" + pageToken
		},
		func(response *testPage) ([]string, string) { return response.Items, response.NextPageToken },
		[]fetch_config.Option{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"a", "b", "c"}
	if len(items) != len(expected) {
		t.Fatalf("expected %d items, got %d", len(expected), len(items))
	}
	for i, item := range items {
		if item != expected[i] {
			t.Errorf("expected item %d to be %q, got %q", i, expected[i], item)
		}
	}
}
