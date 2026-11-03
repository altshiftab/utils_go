package artifact_registry

import (
	"context"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/artifact_registry/artifact_registry_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/artifact_registry/types/descriptor"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/artifact_registry/types/index"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/artifact_registry/types/manifest"
	altshiftTestingCmp "github.com/altshiftab/utils_go/pkg/testing/cmp"
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
		if accept := r.Header.Get("Accept"); accept != MediaTypeOciIndex {
			t.Errorf("Accept = %q, want %q", accept, MediaTypeOciIndex)
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

// fakeRegistry models what a client relies on of the distribution API: an
// upload session handed out by a POST and closed by a PUT naming the digest,
// which the registry checks against what arrived; manifests stored under their
// own digest; and, when it implements the referrers API, the OCI-Subject header
// on a manifest naming a subject.
type fakeRegistry struct {
	t *testing.T

	mutex     sync.Mutex
	blobs     map[string][]byte
	manifests map[string][]byte
	// manifestTypes is the media type each manifest was stored as.
	manifestTypes map[string]string
	// putQueries are the raw queries the upload PUTs arrived with.
	putQueries []string

	// location overrides where the upload session is said to be.
	location string
	// withoutReferrers makes the registry one that does not state OCI-Subject.
	withoutReferrers bool
	// storedDigest overrides the digest the registry says it stored.
	storedDigest string
}

func newFakeRegistry(t *testing.T) *fakeRegistry {
	return &fakeRegistry{
		t:             t,
		blobs:         make(map[string][]byte),
		manifests:     make(map[string][]byte),
		manifestTypes: make(map[string]string),
	}
}

func (f *fakeRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Errorf("read body: %v", err)
	}

	const name = "/v2/my-project/my-repo/my-image"

	switch {
	case r.Method == http.MethodPost && r.URL.Path == name+"/blobs/uploads/":
		location := f.location
		if location == "" {
			location = name + "/blobs/uploads/session-1?_state=opaque%2Bstate"
		}
		w.Header().Set("Location", location)
		w.WriteHeader(http.StatusAccepted)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, name+"/blobs/uploads/"):
		f.putQueries = append(f.putQueries, r.URL.RawQuery)
		digest := r.URL.Query().Get("digest")
		if Digest(body) != digest {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.blobs[digest] = body
		stored := digest
		if f.storedDigest != "" {
			stored = f.storedDigest
		}
		w.Header().Set("Docker-Content-Digest", stored)
		w.WriteHeader(http.StatusCreated)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, name+"/manifests/"):
		digest := Digest(body)
		f.manifests[digest] = body
		f.manifestTypes[digest] = r.Header.Get("Content-Type")

		var stored manifest.Manifest
		if err := json.Unmarshal(body, &stored); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if stored.Subject != nil && !f.withoutReferrers {
			w.Header().Set("OCI-Subject", stored.Subject.Digest)
		}
		if f.storedDigest != "" {
			digest = f.storedDigest
		}
		w.Header().Set("Docker-Content-Digest", digest)
		w.WriteHeader(http.StatusCreated)
	case r.Method == http.MethodHead && strings.HasPrefix(r.URL.Path, name+"/manifests/"):
		reference := strings.TrimPrefix(r.URL.Path, name+"/manifests/")
		data, ok := f.manifests[reference]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", f.manifestTypes[reference])
		w.Header().Set("Docker-Content-Digest", reference)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
	default:
		f.t.Errorf("unexpected registry request: %s %s", r.Method, r.URL)
		w.WriteHeader(http.StatusNotFound)
	}
}

func fakeRegistryClient(t *testing.T, registry *fakeRegistry) (*Client, *url.URL) {
	t.Helper()
	server := httptest.NewServer(registry)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return NewClient("", artifact_registry_config.WithBaseUrl(u)), u
}

func TestDigest(t *testing.T) {
	t.Parallel()

	// The empty descriptor's well-known address, which every OCI artifact without
	// a config names.
	if got, want := Digest([]byte("{}")), "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"; got != want {
		t.Errorf("Digest({}) = %q, want %q", got, want)
	}
}

func TestPutBlob(t *testing.T) {
	t.Parallel()

	data := []byte(`{"payloadType":"application/vnd.in-toto+json"}`)

	testCases := []struct {
		name     string
		location func(base *url.URL) string
		// wantQuery is what the closing PUT is sent with: the session's own query,
		// as the registry handed it out, and the digest after it.
		wantQuery string
	}{
		{
			name:      "a relative location",
			location:  func(*url.URL) string { return "" },
			wantQuery: "_state=opaque%2Bstate&digest=" + url.QueryEscape(Digest(data)),
		},
		{
			name: "an absolute location on the same host",
			location: func(base *url.URL) string {
				return base.String() + "/v2/my-project/my-repo/my-image/blobs/uploads/session-2?_state=x"
			},
			wantQuery: "_state=x&digest=" + url.QueryEscape(Digest(data)),
		},
		{
			name: "a location without a query",
			location: func(*url.URL) string {
				return "/v2/my-project/my-repo/my-image/blobs/uploads/session-3"
			},
			wantQuery: "digest=" + url.QueryEscape(Digest(data)),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			registry := newFakeRegistry(t)
			client, base := fakeRegistryClient(t, registry)
			registry.location = testCase.location(base)

			digest, err := client.PutBlob(context.Background(), "my-project/my-repo/my-image", data)
			if err != nil {
				t.Fatalf("put blob: %v", err)
			}

			if digest != Digest(data) {
				t.Errorf("digest = %q, want %q", digest, Digest(data))
			}
			if stored := registry.blobs[digest]; string(stored) != string(data) {
				t.Errorf("stored = %q, want %q", stored, data)
			}
			if len(registry.putQueries) != 1 {
				t.Fatalf("upload puts = %d, want 1", len(registry.putQueries))
			}
			if registry.putQueries[0] != testCase.wantQuery {
				t.Errorf("upload query = %q, want %q", registry.putQueries[0], testCase.wantQuery)
			}
		})
	}
}

// The location of an upload session is followed with the caller's credentials,
// so one on another host is refused rather than sent the content.
func TestPutBlobRefusesAnotherHost(t *testing.T) {
	t.Parallel()

	registry := newFakeRegistry(t)
	client, _ := fakeRegistryClient(t, registry)
	registry.location = "https://elsewhere.example/v2/upload"

	if _, err := client.PutBlob(context.Background(), "my-project/my-repo/my-image", []byte("x")); err == nil {
		t.Fatal("expected an error for an upload location on another host")
	}
	if len(registry.putQueries) != 0 {
		t.Errorf("the content was sent somewhere: %v", registry.putQueries)
	}
}

func TestPutBlobRefusesAnotherDigest(t *testing.T) {
	t.Parallel()

	registry := newFakeRegistry(t)
	registry.storedDigest = "sha256:0000"
	client, _ := fakeRegistryClient(t, registry)

	if _, err := client.PutBlob(context.Background(), "my-project/my-repo/my-image", []byte("x")); err == nil {
		t.Fatal("expected an error when the registry stored the blob under another digest")
	}
}

func TestPutManifest(t *testing.T) {
	t.Parallel()

	subject := &descriptor.Descriptor{
		MediaType: MediaTypeDockerManifest,
		Digest:    "sha256:abc123",
		Size:      1234,
	}

	testCases := []struct {
		name             string
		subject          *descriptor.Descriptor
		withoutReferrers bool
		wantSubject      string
	}{
		{name: "a referrer is indexed", subject: subject, wantSubject: subject.Digest},
		{name: "a registry without the referrers API", subject: subject, withoutReferrers: true},
		{name: "a manifest naming no subject"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			registry := newFakeRegistry(t)
			registry.withoutReferrers = testCase.withoutReferrers
			client, _ := fakeRegistryClient(t, registry)

			data, err := json.Marshal(&manifest.Manifest{
				SchemaVersion: 2,
				MediaType:     MediaTypeOciManifest,
				ArtifactType:  "application/vnd.dsse.envelope.v1+json",
				Subject:       testCase.subject,
			})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			digest, gotSubject, err := client.PutManifest(
				context.Background(), "my-project/my-repo/my-image", Digest(data), MediaTypeOciManifest, data,
			)
			if err != nil {
				t.Fatalf("put manifest: %v", err)
			}

			if digest != Digest(data) {
				t.Errorf("digest = %q, want %q", digest, Digest(data))
			}
			if gotSubject != testCase.wantSubject {
				t.Errorf("subject = %q, want %q", gotSubject, testCase.wantSubject)
			}
			if got := registry.manifestTypes[digest]; got != MediaTypeOciManifest {
				t.Errorf("stored as %q, want %q", got, MediaTypeOciManifest)
			}
		})
	}
}

func TestPutManifestRefusesAnotherDigest(t *testing.T) {
	t.Parallel()

	registry := newFakeRegistry(t)
	registry.storedDigest = "sha256:0000"
	client, _ := fakeRegistryClient(t, registry)

	if _, _, err := client.PutManifest(
		context.Background(), "my-project/my-repo/my-image", "latest", MediaTypeOciManifest, []byte(`{}`),
	); err == nil {
		t.Fatal("expected an error when the registry stored the manifest under another digest")
	}
}

func TestPutArgumentErrors(t *testing.T) {
	t.Parallel()

	client := NewClient("us")

	testCases := []struct {
		name string
		call func() error
	}{
		{name: "blob without a name", call: func() error {
			_, err := client.PutBlob(context.Background(), "", []byte("x"))
			return err
		}},
		{name: "manifest without a name", call: func() error {
			_, _, err := client.PutManifest(context.Background(), "", "latest", MediaTypeOciManifest, []byte("{}"))
			return err
		}},
		{name: "manifest without a reference", call: func() error {
			_, _, err := client.PutManifest(context.Background(), "p/r/i", "", MediaTypeOciManifest, []byte("{}"))
			return err
		}},
		{name: "manifest without a media type", call: func() error {
			_, _, err := client.PutManifest(context.Background(), "p/r/i", "latest", "", []byte("{}"))
			return err
		}},
		{name: "manifest without content", call: func() error {
			_, _, err := client.PutManifest(context.Background(), "p/r/i", "latest", MediaTypeOciManifest, nil)
			return err
		}},
		{name: "head without a name", call: func() error {
			_, err := client.HeadManifest(context.Background(), "", "latest")
			return err
		}},
		{name: "head without a reference", call: func() error {
			_, err := client.HeadManifest(context.Background(), "p/r/i", "")
			return err
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if err := testCase.call(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestHeadManifest(t *testing.T) {
	t.Parallel()

	data := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json"}`)

	var accept string
	registry := newFakeRegistry(t)
	registry.manifests[Digest(data)] = data
	registry.manifestTypes[Digest(data)] = MediaTypeDockerManifest + "; charset=utf-8"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept = r.Header.Get("Accept")
		registry.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	client := NewClient("", artifact_registry_config.WithBaseUrl(u))

	got, err := client.HeadManifest(context.Background(), "my-project/my-repo/my-image", Digest(data))
	if err != nil {
		t.Fatalf("head manifest: %v", err)
	}

	want := &descriptor.Descriptor{MediaType: MediaTypeDockerManifest, Digest: Digest(data), Size: int64(len(data))}
	if diff := altshiftTestingCmp.Diff(want, got); diff != "" {
		t.Errorf("descriptor mismatch (-want +got):\n%s", diff)
	}

	// An image pushed by Docker is described by a Docker manifest, and asking for
	// an OCI one only would have it refused.
	for _, mediaType := range []string{MediaTypeOciManifest, MediaTypeDockerManifest, MediaTypeOciIndex, MediaTypeDockerManifestList} {
		if !strings.Contains(accept, mediaType) {
			t.Errorf("accept %q does not name %q", accept, mediaType)
		}
	}
}

func TestHeadManifestNotFound(t *testing.T) {
	t.Parallel()

	client, _ := fakeRegistryClient(t, newFakeRegistry(t))

	if _, err := client.HeadManifest(context.Background(), "my-project/my-repo/my-image", "sha256:missing"); err == nil {
		t.Fatal("expected an error for a manifest that is not there")
	}
}
