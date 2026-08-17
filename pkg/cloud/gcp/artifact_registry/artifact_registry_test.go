package artifact_registry

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/artifact_registry/artifact_registry_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/artifact_registry/types/descriptor"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/artifact_registry/types/index"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/artifact_registry/types/manifest"
)

func testServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return NewClient("", artifact_registry_config.WithBaseUrl(u))
}

func TestGetManifest(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/v2/my-project/my-repo/my-image/manifests/latest") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if !strings.Contains(r.Header.Get("Accept"), "application/vnd.oci.image.manifest.v1+json") {
			t.Errorf("unexpected Accept header: %s", r.Header.Get("Accept"))
		}

		w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		w.Header().Set("Docker-Content-Digest", "sha256:abc123")
		if err := json.MarshalWrite(w, &manifest.Manifest{
			SchemaVersion: 2,
			MediaType:     "application/vnd.oci.image.manifest.v1+json",
			Config: &descriptor.Descriptor{
				MediaType: "application/vnd.oci.image.config.v1+json",
				Digest:    "sha256:config123",
				Size:      512,
			},
			Layers: []*descriptor.Descriptor{
				{
					MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					Digest:    "sha256:layer123",
					Size:      1024,
				},
			},
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	digest, m, err := client.GetManifest(context.Background(), "my-project/my-repo/my-image", "latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if digest != "sha256:abc123" {
		t.Errorf("expected digest 'sha256:abc123', got %q", digest)
	}
	if m.SchemaVersion != 2 {
		t.Errorf("expected schema version 2, got %d", m.SchemaVersion)
	}
	if m.Config == nil {
		t.Fatal("expected config descriptor")
	}
	if m.Config.Digest != "sha256:config123" {
		t.Errorf("expected config digest 'sha256:config123', got %q", m.Config.Digest)
	}
	if len(m.Layers) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(m.Layers))
	}
	if m.Layers[0].Digest != "sha256:layer123" {
		t.Errorf("expected layer digest 'sha256:layer123', got %q", m.Layers[0].Digest)
	}
}

func TestGetManifest_EmptyName(t *testing.T) {
	t.Parallel()

	client := NewClient("us")
	_, _, err := client.GetManifest(context.Background(), "", "latest")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestGetManifest_EmptyReference(t *testing.T) {
	t.Parallel()

	client := NewClient("us")
	_, _, err := client.GetManifest(context.Background(), "project/repo/image", "")
	if err == nil {
		t.Fatal("expected error for empty reference")
	}
}

func TestGetManifest_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient("us")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := client.GetManifest(ctx, "project/repo/image", "latest")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestListReferrers(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/v2/my-project/my-repo/my-image/referrers/sha256:abc123") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
		if err := json.MarshalWrite(w, &index.Index{
			SchemaVersion: 2,
			MediaType:     "application/vnd.oci.image.index.v1+json",
			Manifests: []*descriptor.Descriptor{
				{
					MediaType:    "application/vnd.oci.image.manifest.v1+json",
					Digest:       "sha256:sbom456",
					Size:         256,
					ArtifactType: "application/spdx+json",
				},
			},
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	idx, err := client.ListReferrers(context.Background(), "my-project/my-repo/my-image", "sha256:abc123", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(idx.Manifests) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(idx.Manifests))
	}
	if idx.Manifests[0].ArtifactType != "application/spdx+json" {
		t.Errorf("expected artifact type 'application/spdx+json', got %q", idx.Manifests[0].ArtifactType)
	}
	if idx.Manifests[0].Digest != "sha256:sbom456" {
		t.Errorf("expected digest 'sha256:sbom456', got %q", idx.Manifests[0].Digest)
	}
}

func TestListReferrers_WithArtifactTypeFilter(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("artifactType") != "application/spdx+json" {
			t.Errorf("expected artifactType filter, got %q", r.URL.Query().Get("artifactType"))
		}

		w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
		if err := json.MarshalWrite(w, &index.Index{
			SchemaVersion: 2,
			MediaType:     "application/vnd.oci.image.index.v1+json",
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	_, err := client.ListReferrers(context.Background(), "my-project/my-repo/my-image", "sha256:abc123", "application/spdx+json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListReferrers_EmptyName(t *testing.T) {
	t.Parallel()

	client := NewClient("us")
	_, err := client.ListReferrers(context.Background(), "", "sha256:abc", "")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestListReferrers_EmptyDigest(t *testing.T) {
	t.Parallel()

	client := NewClient("us")
	_, err := client.ListReferrers(context.Background(), "project/repo/image", "", "")
	if err == nil {
		t.Fatal("expected error for empty digest")
	}
}

func TestListReferrers_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient("us")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.ListReferrers(ctx, "project/repo/image", "sha256:abc", "")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestGetBlob(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/v2/my-project/my-repo/my-image/blobs/sha256:layer123") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		if _, err := w.Write([]byte(`{"spdxVersion":"SPDX-2.3"}`)); err != nil {
			t.Errorf("write: %v", err)
		}
	})

	data, err := client.GetBlob(context.Background(), "my-project/my-repo/my-image", "sha256:layer123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `{"spdxVersion":"SPDX-2.3"}` {
		t.Errorf("unexpected blob content: %q", string(data))
	}
}

func TestGetBlob_EmptyName(t *testing.T) {
	t.Parallel()

	client := NewClient("us")
	_, err := client.GetBlob(context.Background(), "", "sha256:abc")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestGetBlob_EmptyDigest(t *testing.T) {
	t.Parallel()

	client := NewClient("us")
	_, err := client.GetBlob(context.Background(), "project/repo/image", "")
	if err == nil {
		t.Fatal("expected error for empty digest")
	}
}

func TestGetBlob_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient("us")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.GetBlob(ctx, "project/repo/image", "sha256:abc")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestNewClient(t *testing.T) {
	t.Parallel()

	client := NewClient("us")
	if client.baseUrl.Host != "us-docker.pkg.dev" {
		t.Errorf("expected host 'us-docker.pkg.dev', got %q", client.baseUrl.Host)
	}
	if client.baseUrl.Scheme != "https" {
		t.Errorf("expected scheme 'https', got %q", client.baseUrl.Scheme)
	}
	if client.baseUrl.Path != "/v2/" {
		t.Errorf("expected path '/v2/', got %q", client.baseUrl.Path)
	}
}
