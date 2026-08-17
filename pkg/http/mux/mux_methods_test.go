package mux

import (
	"net/http"
	"testing"

	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	staticContentPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint/static_content"
	muxResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
)

func TestNew(t *testing.T) {
	t.Parallel()

	endpoint := &endpointPkg.Endpoint{Path: "/x", Method: http.MethodGet}
	mux := New(endpoint)

	if mux.DefaultHeaders == nil {
		t.Error("expected default headers to be set")
	}
	if mux.DefaultDocumentHeaders == nil {
		t.Error("expected default document headers to be set")
	}
	if mux.Get("/x", http.MethodGet) != endpoint {
		t.Error("expected the endpoint to be registered")
	}
}

func TestMux_Get(t *testing.T) {
	t.Parallel()

	endpoint := &endpointPkg.Endpoint{Path: "/x", Method: http.MethodGet}
	mux := New(endpoint)

	if mux.Get("/x", "get") != endpoint {
		t.Error("expected a case-insensitive method lookup")
	}
	if mux.Get("/missing", http.MethodGet) != nil {
		t.Error("expected nil for a missing path")
	}
	if mux.Get("/x", http.MethodPost) != nil {
		t.Error("expected nil for a missing method")
	}
	if (&Mux{}).Get("/x", http.MethodGet) != nil {
		t.Error("expected nil for a nil endpoint map")
	}
}

func TestMux_Delete(t *testing.T) {
	t.Parallel()

	endpoint := &endpointPkg.Endpoint{Path: "/x", Method: http.MethodGet}
	mux := New(endpoint)

	mux.Delete(endpoint)
	if mux.Get("/x", http.MethodGet) != nil {
		t.Error("expected the endpoint to be deleted")
	}
	if _, ok := mux.EndpointMap["/x"]; ok {
		t.Error("expected the empty path entry to be removed")
	}

	// Delete on a mux without an endpoint map is a no-op.
	(&Mux{}).Delete(endpoint)

	// Deleting an unknown path leaves existing endpoints intact.
	other := New(&endpointPkg.Endpoint{Path: "/y", Method: http.MethodGet})
	other.Delete(&endpointPkg.Endpoint{Path: "/unknown", Method: http.MethodGet})
	if other.Get("/y", http.MethodGet) == nil {
		t.Error("expected /y to remain after deleting an unknown path")
	}
}

func TestMux_AddEdgeCases(t *testing.T) {
	t.Parallel()

	mux := &Mux{}
	// Empty args, a nil endpoint, empty method/path, and a non-public endpoint
	// without an authentication parser all exercise the warning/skip branches.
	mux.Add()
	mux.Add(
		nil,
		&endpointPkg.Endpoint{Path: "", Method: ""},
		&endpointPkg.Endpoint{Path: "/private", Method: http.MethodGet, Public: false},
	)

	if mux.Get("/private", http.MethodGet) == nil {
		t.Error("expected the non-public endpoint to still be registered")
	}
}

func TestMux_GetDocumentEndpointSpecifications(t *testing.T) {
	t.Parallel()

	htmlEndpoint := &endpointPkg.Endpoint{
		Path:   "/page",
		Method: http.MethodGet,
		StaticContent: &staticContentPkg.StaticContent{
			StaticContentData: staticContentPkg.StaticContentData{
				Headers: []*muxResponse.HeaderEntry{{Name: "Content-Type", Value: "text/html"}},
			},
		},
	}
	cssEndpoint := &endpointPkg.Endpoint{
		Path:   "/style.css",
		Method: http.MethodGet,
		StaticContent: &staticContentPkg.StaticContent{
			StaticContentData: staticContentPkg.StaticContentData{
				Headers: []*muxResponse.HeaderEntry{{Name: "Content-Type", Value: "text/css"}},
			},
		},
	}
	handlerEndpoint := &endpointPkg.Endpoint{Path: "/api", Method: http.MethodGet}

	mux := New(htmlEndpoint, cssEndpoint, handlerEndpoint)

	documents := mux.GetDocumentEndpointSpecifications()
	if len(documents) != 1 || documents[0] != htmlEndpoint {
		t.Fatalf("expected only the html endpoint, got %#v", documents)
	}
}

func TestMux_DuplicateEndpointSpecification(t *testing.T) {
	t.Parallel()

	if err := (&Mux{}).DuplicateEndpointSpecification(nil); err == nil {
		t.Fatal("expected an error for a nil endpoint")
	}

	endpoint := &endpointPkg.Endpoint{Path: "/original", Method: http.MethodGet}
	mux := New(endpoint)

	if err := mux.DuplicateEndpointSpecification(endpoint, "/copy-1", "/copy-2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mux.Get("/copy-1", http.MethodGet) == nil || mux.Get("/copy-2", http.MethodGet) == nil {
		t.Fatal("expected the duplicated routes to be registered")
	}
}

func TestMux_ContentSecurityPolicy(t *testing.T) {
	t.Parallel()

	mux := New()

	csp, err := mux.GetContentSecurityPolicy()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if csp == nil {
		t.Fatal("expected a default content security policy")
	}

	if err := mux.SetContentSecurityPolicy(csp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if roundTripped, err := mux.GetContentSecurityPolicy(); err != nil || roundTripped == nil {
		t.Fatalf("expected the policy to round-trip, got %#v (err %v)", roundTripped, err)
	}

	if err := mux.SetContentSecurityPolicy(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cleared, err := mux.GetContentSecurityPolicy(); err != nil || cleared != nil {
		t.Fatalf("expected the policy to be cleared, got %#v (err %v)", cleared, err)
	}

	if err := (&Mux{}).SetContentSecurityPolicy(csp); err == nil {
		t.Fatal("expected an error when default document headers are nil")
	}
}
