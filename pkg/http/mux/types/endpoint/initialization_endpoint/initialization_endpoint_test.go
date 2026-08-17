package initialization_endpoint

import (
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
)

func TestEndpoint_EmbeddedFieldPromotion(t *testing.T) {
	t.Parallel()

	initEndpoint := &Endpoint{
		Endpoint: &endpoint.Endpoint{
			Path:   "/health",
			Method: http.MethodGet,
		},
		Initialized: true,
	}

	// Fields promoted from the embedded *endpoint.Endpoint.
	if initEndpoint.Path != "/health" {
		t.Errorf("promoted Path = %q, want %q", initEndpoint.Path, "/health")
	}
	if initEndpoint.Method != http.MethodGet {
		t.Errorf("promoted Method = %q, want %q", initEndpoint.Method, http.MethodGet)
	}
	if !initEndpoint.Initialized {
		t.Error("Initialized = false, want true")
	}
}

func TestEndpoint_InitializedDefaultsFalse(t *testing.T) {
	t.Parallel()

	initEndpoint := &Endpoint{}

	if initEndpoint.Initialized {
		t.Error("Initialized = true, want false by default")
	}
}

func TestEndpoint_EmbeddedPointerAccessible(t *testing.T) {
	t.Parallel()

	inner := &endpoint.Endpoint{Path: "/inner", Method: http.MethodPost}
	initEndpoint := &Endpoint{Endpoint: inner}

	if initEndpoint.Endpoint != inner {
		t.Error("embedded Endpoint pointer does not match the assigned value")
	}
	if initEndpoint.Method != http.MethodPost {
		t.Errorf("promoted Method = %q, want %q", initEndpoint.Method, http.MethodPost)
	}
}
