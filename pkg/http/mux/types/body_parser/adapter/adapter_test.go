package adapter

import (
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/body_parser/json_body_parser"
)

type payload struct {
	Name string `json:"name"`
}

func TestAdapter_Parse(t *testing.T) {
	t.Parallel()

	adapter := New[payload](json_body_parser.New[payload]())

	result, responseError := adapter.Parse(nil, []byte(`{"name":"alice"}`))
	if responseError != nil {
		t.Fatalf("unexpected error: %#v", responseError)
	}

	parsed, ok := result.(payload)
	if !ok {
		t.Fatalf("expected a payload, got %T", result)
	}
	if parsed.Name != "alice" {
		t.Fatalf("got %q, want alice", parsed.Name)
	}
}
