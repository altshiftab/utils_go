package blob_storage

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/azure/blob_storage/list_blobs_config"
	"github.com/altshiftab/utils_go/pkg/cloud/azure/blob_storage/upload_config"
)

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()

	data := make([]byte, n)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand read: %v", err)
	}

	return data
}

func readBlob(t *testing.T, client *Client, container string, blob string, versionId string) *Download {
	t.Helper()

	download, err := client.GetBlob(t.Context(), container, blob, versionId)
	if err != nil {
		t.Fatalf("get blob: %v", err)
	}

	return download
}

func readAll(t *testing.T, download *Download) []byte {
	t.Helper()

	defer func() { _ = download.Body.Close() }()
	content, err := io.ReadAll(download.Body)
	if err != nil {
		t.Fatalf("io read all: %v", err)
	}

	return content
}

func TestNewClient(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		account string
		wantErr bool
	}{
		{name: "a valid name", account: "intilsignalsbackupdev"},
		{name: "the shortest", account: "abc"},
		{name: "empty", account: "", wantErr: true},
		{name: "too short", account: "ab", wantErr: true},
		{name: "too long", account: "abcdefghijklmnopqrstuvwxy", wantErr: true},
		{name: "upper case", account: "Intil", wantErr: true},
		{name: "a hyphen", account: "intil-backup", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client, err := NewClient(testCase.account)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("new client: %v, want error %t", err, testCase.wantErr)
			}
			if err == nil && client.baseUrl.Host != testCase.account+"."+DomainSuffix {
				t.Errorf("host %q", client.baseUrl.Host)
			}
		})
	}
}

func TestUploadRoundTrip(t *testing.T) {
	t.Parallel()

	const blockSize = 1024

	testCases := []struct {
		name string
		size int
		blob string
	}{
		{name: "empty", size: 0, blob: "empty"},
		{name: "one byte", size: 1, blob: "one"},
		{name: "one block exactly", size: blockSize, blob: "block"},
		{name: "a block and a byte", size: blockSize + 1, blob: "block-and-byte"},
		{name: "several blocks", size: 3*blockSize + 17, blob: "several"},
		{name: "a name with slashes, spaces and escapes", size: 10, blob: "a/b c/100%/ä.tar.age"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, client := newFakeBlobService(t)
			content := randomBytes(t, testCase.size)
			metadata := map[string]string{"ciphertextsha256": "abc123", "source": "export"}

			commit, err := client.Upload(
				t.Context(), "databases", testCase.blob, bytes.NewReader(content),
				upload_config.WithBlockSize(blockSize),
				upload_config.WithContentType("application/octet-stream"),
				upload_config.WithMetadata(metadata),
			)
			if err != nil {
				t.Fatalf("upload: %v", err)
			}
			if commit.VersionId == "" {
				t.Error("no version id")
			}

			download := readBlob(t, client, "databases", testCase.blob, "")
			if got := readAll(t, download); !bytes.Equal(got, content) {
				t.Errorf("read back %d bytes that differ from the %d uploaded", len(got), len(content))
			}
			if download.VersionId != commit.VersionId {
				t.Errorf("read version %q, committed %q", download.VersionId, commit.VersionId)
			}
			if download.ContentType != "application/octet-stream" {
				t.Errorf("content type %q", download.ContentType)
			}
			for name, value := range metadata {
				if download.Metadata[name] != value {
					t.Errorf("metadata %q is %q, want %q", name, download.Metadata[name], value)
				}
			}
		})
	}
}

func TestUploadIfAbsent(t *testing.T) {
	t.Parallel()

	_, client := newFakeBlobService(t)
	first := []byte("first")
	second := []byte("second")

	if _, err := client.Upload(t.Context(), "databases", "dump", bytes.NewReader(first), upload_config.WithIfAbsent(true)); err != nil {
		t.Fatalf("upload (first): %v", err)
	}

	_, err := client.Upload(t.Context(), "databases", "dump", bytes.NewReader(second), upload_config.WithIfAbsent(true))
	if !errors.Is(err, ErrBlobExists) {
		t.Fatalf("upload (again, if absent): %v, want %v", err, ErrBlobExists)
	}
	if got := readAll(t, readBlob(t, client, "databases", "dump", "")); !bytes.Equal(got, first) {
		t.Errorf("a refused upload changed the blob to %q", got)
	}

	// Without the condition, the same name takes a new version and keeps the old one.
	if _, err := client.Upload(t.Context(), "databases", "dump", bytes.NewReader(second)); err != nil {
		t.Fatalf("upload (overwrite): %v", err)
	}
	if got := readAll(t, readBlob(t, client, "databases", "dump", "")); !bytes.Equal(got, second) {
		t.Errorf("current version %q, want %q", got, second)
	}

	versions, err := client.ListAllBlobs(t.Context(), "databases", list_blobs_config.WithIncludeVersions(true))
	if err != nil {
		t.Fatalf("list all blobs: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("listed %d versions, want 2", len(versions))
	}
	if versions[0].IsCurrentVersion || !versions[1].IsCurrentVersion {
		t.Errorf("current flags %t, %t, want false, true", versions[0].IsCurrentVersion, versions[1].IsCurrentVersion)
	}
	if got := readAll(t, readBlob(t, client, "databases", "dump", versions[0].VersionId)); !bytes.Equal(got, first) {
		t.Errorf("earlier version %q, want %q", got, first)
	}
}

func TestListAllBlobs(t *testing.T) {
	t.Parallel()

	_, client := newFakeBlobService(t)
	for _, name := range []string{"prod/a", "prod/b", "prod/c", "prod/d", "prod/e", "dev/a"} {
		if _, err := client.Upload(t.Context(), "databases", name, bytes.NewReader([]byte(name)), upload_config.WithMetadata(map[string]string{"name": name})); err != nil {
			t.Fatalf("upload: %v", err)
		}
	}

	testCases := []struct {
		name       string
		options    []list_blobs_config.Option
		wantNames  []string
		wantFields bool
	}{
		{name: "everything, two to a page", options: []list_blobs_config.Option{list_blobs_config.WithMaxResults(2)}, wantNames: []string{"dev/a", "prod/a", "prod/b", "prod/c", "prod/d", "prod/e"}},
		{name: "a prefix", options: []list_blobs_config.Option{list_blobs_config.WithPrefix("dev/")}, wantNames: []string{"dev/a"}},
		{
			name:       "with metadata",
			options:    []list_blobs_config.Option{list_blobs_config.WithPrefix("prod/a"), list_blobs_config.WithIncludeMetadata(true)},
			wantNames:  []string{"prod/a"},
			wantFields: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			blobs, err := client.ListAllBlobs(t.Context(), "databases", testCase.options...)
			if err != nil {
				t.Fatalf("list all blobs: %v", err)
			}

			names := make([]string, 0, len(blobs))
			for _, blob := range blobs {
				names = append(names, blob.Name)
				if testCase.wantFields && blob.Metadata.Map()["name"] != blob.Name {
					t.Errorf("%s: metadata %v", blob.Name, blob.Metadata.Map())
				}
			}
			if !equalStrings(names, testCase.wantNames) {
				t.Errorf("listed %v, want %v", names, testCase.wantNames)
			}
		})
	}
}

func equalStrings(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func TestServiceError(t *testing.T) {
	t.Parallel()

	_, client := newFakeBlobService(t)

	_, err := client.GetBlob(t.Context(), "databases", "missing", "")
	serviceErr, ok := errors.AsType[*ServiceError](err)
	if !ok {
		t.Fatalf("get blob: %v, want a service error", err)
	}
	if serviceErr.StatusCode != http.StatusNotFound || serviceErr.Code != "BlobNotFound" {
		t.Errorf("status %d, code %q", serviceErr.StatusCode, serviceErr.Code)
	}
	if errors.Is(err, ErrBlobExists) {
		t.Error("a missing blob reported as existing")
	}
}

func TestUploadRejects(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		container string
		blob      string
		src       io.Reader
		options   []upload_config.Option
	}{
		{name: "no source", container: "databases", blob: "x", src: nil},
		{name: "no container", container: "", blob: "x", src: bytes.NewReader(nil)},
		{name: "no blob", container: "databases", blob: "", src: bytes.NewReader(nil)},
		{name: "a block size of zero", container: "databases", blob: "x", src: bytes.NewReader(nil), options: []upload_config.Option{upload_config.WithBlockSize(0)}},
		{name: "a block size past the limit", container: "databases", blob: "x", src: bytes.NewReader(nil), options: []upload_config.Option{upload_config.WithBlockSize(upload_config.MaxBlockSize + 1)}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			service, client := newFakeBlobService(t)
			if _, err := client.Upload(t.Context(), testCase.container, testCase.blob, testCase.src, testCase.options...); err == nil {
				t.Error("upload: no error")
			}

			service.mu.Lock()
			defer service.mu.Unlock()
			if service.requests != 0 {
				t.Errorf("%d requests made for an upload refused before any", service.requests)
			}
		})
	}
}
