package cloud_storage

import (
	"context"
	"encoding/hex"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_storage/cloud_storage_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_storage/types/bucket"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_storage/types/object"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_storage/types/object_list"
)

// fakeSigner records the payload it was asked to sign and returns a fixed signature.
type fakeSigner struct {
	email      string
	signature  []byte
	gotPayload []byte
}

func (f *fakeSigner) Email() string { return f.email }

func (f *fakeSigner) Sign(_ context.Context, payload []byte) ([]byte, error) {
	f.gotPayload = append(f.gotPayload[:0], payload...)
	return f.signature, nil
}

func testServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return NewClient(cloud_storage_config.WithBaseUrl(u))
}

func TestInsertBucket(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/storage/v1/b") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("project") != "my-project" {
			t.Errorf("unexpected project: %s", r.URL.Query().Get("project"))
		}

		var input bucket.Bucket
		if err := json.UnmarshalRead(r.Body, &input); err != nil {
			t.Errorf("decode: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &bucket.Bucket{
			Kind:         "storage#bucket",
			Name:         input.Name,
			Location:     "US",
			StorageClass: "STANDARD",
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	b, err := client.InsertBucket(context.Background(), "my-project", &bucket.Bucket{Name: "test-bucket"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.Name != "test-bucket" {
		t.Errorf("expected bucket name 'test-bucket', got %q", b.Name)
	}
	if b.Kind != "storage#bucket" {
		t.Errorf("expected kind 'storage#bucket', got %q", b.Kind)
	}
}

func TestInsertBucket_EmptyProject(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.InsertBucket(context.Background(), "", &bucket.Bucket{Name: "b"})
	if err == nil {
		t.Fatal("expected error for empty project")
	}
}

func TestInsertBucket_NilConfig(t *testing.T) {
	t.Parallel()

	client := NewClient()
	b, err := client.InsertBucket(context.Background(), "project", nil)
	if err == nil {
		t.Fatal("expected an error for nil config")
	}
	if b != nil {
		t.Error("expected nil bucket for nil config")
	}
}

func TestInsertBucket_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.InsertBucket(ctx, "project", &bucket.Bucket{Name: "b"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestPatchBucket(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/b/my-bucket") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &bucket.Bucket{
			Name:         "my-bucket",
			StorageClass: "NEARLINE",
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	b, err := client.PatchBucket(context.Background(), "my-bucket", &bucket.Bucket{StorageClass: "NEARLINE"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.StorageClass != "NEARLINE" {
		t.Errorf("expected storage class 'NEARLINE', got %q", b.StorageClass)
	}
}

func TestPatchBucket_EmptyName(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.PatchBucket(context.Background(), "", &bucket.Bucket{})
	if err == nil {
		t.Fatal("expected error for empty bucket name")
	}
}

func TestPatchBucket_NilConfig(t *testing.T) {
	t.Parallel()

	client := NewClient()
	b, err := client.PatchBucket(context.Background(), "bucket", nil)
	if err == nil {
		t.Fatal("expected an error for nil config")
	}
	if b != nil {
		t.Error("expected nil bucket for nil config")
	}
}

func TestGetObject(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/b/my-bucket/o/my-object.txt") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &object.Object{
			Kind:         "storage#object",
			Name:         "my-object.txt",
			Bucket:       "my-bucket",
			Size:         "1024",
			StorageClass: "STANDARD",
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	obj, err := client.GetObject(context.Background(), "my-bucket", "my-object.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obj.Name != "my-object.txt" {
		t.Errorf("expected object name 'my-object.txt', got %q", obj.Name)
	}
	if obj.Bucket != "my-bucket" {
		t.Errorf("expected bucket 'my-bucket', got %q", obj.Bucket)
	}
}

func TestGetObject_EmptyBucketName(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.GetObject(context.Background(), "", "obj")
	if err == nil {
		t.Fatal("expected error for empty bucket name")
	}
}

func TestGetObject_EmptyObjectName(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.GetObject(context.Background(), "bucket", "")
	if err == nil {
		t.Fatal("expected error for empty object name")
	}
}

func TestDownloadObject(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Query().Get("alt") != "media" {
			t.Errorf("expected alt=media, got %q", r.URL.Query().Get("alt"))
		}
		if _, err := w.Write([]byte("file content here")); err != nil {
			t.Errorf("write: %v", err)
		}
	})

	data, err := client.DownloadObject(context.Background(), "my-bucket", "my-file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "file content here" {
		t.Errorf("expected 'file content here', got %q", string(data))
	}
}

func TestDownloadObject_EmptyBucketName(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.DownloadObject(context.Background(), "", "obj")
	if err == nil {
		t.Fatal("expected error for empty bucket name")
	}
}

func TestDownloadObject_EmptyObjectName(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.DownloadObject(context.Background(), "bucket", "")
	if err == nil {
		t.Fatal("expected error for empty object name")
	}
}

func TestListObjects(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Query().Get("prefix") != "logs/" {
			t.Errorf("unexpected prefix: %s", r.URL.Query().Get("prefix"))
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &object_list.ObjectList{
			Kind: "storage#objects",
			Items: []*object.Object{
				{Name: "logs/2024-01.txt"},
				{Name: "logs/2024-02.txt"},
			},
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	list, err := client.ListObjects(context.Background(), "my-bucket", url.Values{"prefix": {"logs/"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(list.Items))
	}
	if list.Items[0].Name != "logs/2024-01.txt" {
		t.Errorf("unexpected first item: %q", list.Items[0].Name)
	}
}

func TestListObjects_EmptyBucketName(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.ListObjects(context.Background(), "", nil)
	if err == nil {
		t.Fatal("expected error for empty bucket name")
	}
}

func TestListObjects_NilQuery(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &object_list.ObjectList{Kind: "storage#objects"}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	_, err := client.ListObjects(context.Background(), "bucket", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteObject(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/b/my-bucket/o/my-object.txt") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	err := client.DeleteObject(context.Background(), "my-bucket", "my-object.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteObject_EmptyBucketName(t *testing.T) {
	t.Parallel()

	client := NewClient()
	err := client.DeleteObject(context.Background(), "", "obj")
	if err == nil {
		t.Fatal("expected error for empty bucket name")
	}
}

func TestDeleteObject_EmptyObjectName(t *testing.T) {
	t.Parallel()

	client := NewClient()
	err := client.DeleteObject(context.Background(), "bucket", "")
	if err == nil {
		t.Fatal("expected error for empty object name")
	}
}

func TestDeleteObject_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := client.DeleteObject(ctx, "bucket", "obj")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestInsertObject(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/upload/storage/v1/b/my-bucket/o") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("uploadType") != "multipart" {
			t.Errorf("unexpected uploadType: %s", r.URL.Query().Get("uploadType"))
		}

		contentType := r.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "multipart/related") {
			t.Errorf("unexpected content-type: %s", contentType)
		}

		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "hello world") {
			t.Error("request body missing object data")
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &object.Object{
			Kind:   "storage#object",
			Name:   "test.txt",
			Bucket: "my-bucket",
			Size:   "11",
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	obj, err := client.InsertObject(
		context.Background(),
		"my-bucket",
		&object.Object{Name: "test.txt"},
		[]byte("hello world"),
		"text/plain",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obj.Name != "test.txt" {
		t.Errorf("expected object name 'test.txt', got %q", obj.Name)
	}
}

func TestInsertObject_EmptyBucketName(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.InsertObject(context.Background(), "", &object.Object{Name: "o"}, []byte("x"), "text/plain")
	if err == nil {
		t.Fatal("expected error for empty bucket name")
	}
}

func TestInsertObject_EmptyContentType(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.InsertObject(context.Background(), "bucket", &object.Object{Name: "o"}, []byte("x"), "")
	if err == nil {
		t.Fatal("expected error for empty content type")
	}
}

func TestInsertObject_NilMetadata(t *testing.T) {
	t.Parallel()

	client := NewClient()
	obj, err := client.InsertObject(context.Background(), "bucket", nil, []byte("x"), "text/plain")
	if err == nil {
		t.Fatal("expected an error for nil metadata")
	}
	if obj != nil {
		t.Error("expected nil object for nil metadata")
	}
}

func TestInsertObject_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.InsertObject(ctx, "bucket", &object.Object{Name: "o"}, []byte("x"), "text/plain")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestSignedUrl(t *testing.T) {
	t.Parallel()

	client := NewClient()

	signer := &fakeSigner{
		email:     "robot@example.iam.gserviceaccount.com",
		signature: []byte{0xde, 0xad, 0xbe, 0xef},
	}

	urlString, err := client.SignedUrl(
		context.Background(),
		signer,
		http.MethodGet,
		"my-bucket",
		"reports/foo bar.pdf",
		15*time.Minute,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed, err := url.Parse(urlString)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	if parsed.Scheme != "https" || parsed.Host != "storage.googleapis.com" {
		t.Errorf("unexpected scheme/host: %s://%s", parsed.Scheme, parsed.Host)
	}
	// Slashes within the object name must be preserved literally; the space must be %20 (not '+').
	if !strings.HasPrefix(urlString, "https://storage.googleapis.com/my-bucket/reports/foo%20bar.pdf?") {
		t.Errorf("unexpected URL prefix: %s", urlString)
	}

	q := parsed.Query()
	if got := q.Get("X-Goog-Algorithm"); got != "GOOG4-RSA-SHA256" {
		t.Errorf("X-Goog-Algorithm: want GOOG4-RSA-SHA256, got %s", got)
	}
	if got := q.Get("X-Goog-Expires"); got != "900" {
		t.Errorf("X-Goog-Expires: want 900, got %s", got)
	}
	if got := q.Get("X-Goog-SignedHeaders"); got != "host" {
		t.Errorf("X-Goog-SignedHeaders: want host, got %s", got)
	}
	cred := q.Get("X-Goog-Credential")
	if !strings.HasPrefix(cred, signer.email+"/") || !strings.HasSuffix(cred, "/auto/storage/goog4_request") {
		t.Errorf("unexpected X-Goog-Credential: %s", cred)
	}
	if got := q.Get("X-Goog-Signature"); got != hex.EncodeToString(signer.signature) {
		t.Errorf("X-Goog-Signature: want %s, got %s", hex.EncodeToString(signer.signature), got)
	}
	if got := q.Get("X-Goog-Date"); len(got) != len("20060102T150405Z") {
		t.Errorf("X-Goog-Date has unexpected length: %s", got)
	}

	stringToSign := string(signer.gotPayload)
	lines := strings.Split(stringToSign, "\n")
	if len(lines) != 4 {
		t.Fatalf("string-to-sign should have 4 lines, got %d: %q", len(lines), stringToSign)
	}
	if lines[0] != "GOOG4-RSA-SHA256" {
		t.Errorf("string-to-sign line 0: want GOOG4-RSA-SHA256, got %s", lines[0])
	}
	if lines[2] != strings.TrimPrefix(cred, signer.email+"/") {
		t.Errorf("string-to-sign credential scope mismatch: %s vs %s", lines[2], cred)
	}
}

func TestSignedUrl_NilSigner(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.SignedUrl(context.Background(), nil, http.MethodGet, "b", "o", time.Hour)
	if err == nil {
		t.Fatal("expected error for nil signer")
	}
}

func TestSignedUrl_EmptyBucket(t *testing.T) {
	t.Parallel()

	client := NewClient()
	signer := &fakeSigner{email: "x@y.iam.gserviceaccount.com", signature: []byte{1}}
	_, err := client.SignedUrl(context.Background(), signer, http.MethodGet, "", "o", time.Hour)
	if err == nil {
		t.Fatal("expected error for empty bucket")
	}
}

func TestSignedUrl_EmptyObject(t *testing.T) {
	t.Parallel()

	client := NewClient()
	signer := &fakeSigner{email: "x@y.iam.gserviceaccount.com", signature: []byte{1}}
	_, err := client.SignedUrl(context.Background(), signer, http.MethodGet, "b", "", time.Hour)
	if err == nil {
		t.Fatal("expected error for empty object")
	}
}

func TestSignedUrl_ExpiresOutOfRange(t *testing.T) {
	t.Parallel()

	client := NewClient()
	signer := &fakeSigner{email: "x@y.iam.gserviceaccount.com", signature: []byte{1}}

	if _, err := client.SignedUrl(context.Background(), signer, http.MethodGet, "b", "o", 0); err == nil {
		t.Error("expected error for zero expires")
	}
	if _, err := client.SignedUrl(context.Background(), signer, http.MethodGet, "b", "o", 8*24*time.Hour); err == nil {
		t.Error("expected error for expires > 7 days")
	}
}

func TestSignedUrl_DefaultMethod(t *testing.T) {
	t.Parallel()

	client := NewClient()
	signer := &fakeSigner{email: "x@y.iam.gserviceaccount.com", signature: []byte{1}}
	if _, err := client.SignedUrl(context.Background(), signer, "", "b", "o", time.Hour); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stringToSign := string(signer.gotPayload)
	canonicalRequestHash := strings.Split(stringToSign, "\n")[3]
	if canonicalRequestHash == "" {
		t.Error("expected non-empty canonical request hash")
	}
}

func TestInsertObjectIfAbsent(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		statusCode   int
		wantInserted bool
		wantObject   bool
		wantErr      bool
	}{
		{name: "absent", statusCode: http.StatusOK, wantInserted: true, wantObject: true},
		{name: "already taken", statusCode: http.StatusPreconditionFailed},
		{name: "a real failure", statusCode: http.StatusInternalServerError, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var precondition string

			client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
				precondition = r.URL.Query().Get("ifGenerationMatch")

				if testCase.statusCode != http.StatusOK {
					w.WriteHeader(testCase.statusCode)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				if err := json.MarshalWrite(w, &object.Object{Name: "test.txt", Bucket: "my-bucket"}); err != nil {
					t.Errorf("json marshal write: %v", err)
				}
			})

			insertedObject, inserted, err := client.InsertObjectIfAbsent(
				context.Background(),
				"my-bucket",
				&object.Object{Name: "test.txt"},
				[]byte("hello world"),
				"text/plain",
			)

			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("insert object if absent: %v", err)
			}

			// Only if the name is unused means a generation of zero.
			if precondition != "0" {
				t.Errorf("ifGenerationMatch: got %q, want \"0\"", precondition)
			}
			if inserted != testCase.wantInserted {
				t.Errorf("inserted: got %v, want %v", inserted, testCase.wantInserted)
			}
			if (insertedObject != nil) != testCase.wantObject {
				t.Errorf("object: got %v, want present=%v", insertedObject, testCase.wantObject)
			}
		})
	}
}

// The plain insert must not carry the precondition: it replaces by design.
func TestInsertObjectHasNoPrecondition(t *testing.T) {
	t.Parallel()

	var precondition string

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		precondition = r.URL.Query().Get("ifGenerationMatch")
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &object.Object{Name: "test.txt"}); err != nil {
			t.Errorf("json marshal write: %v", err)
		}
	})

	if _, err := client.InsertObject(
		context.Background(),
		"my-bucket",
		&object.Object{Name: "test.txt"},
		[]byte("hello world"),
		"text/plain",
	); err != nil {
		t.Fatalf("insert object: %v", err)
	}

	if precondition != "" {
		t.Errorf("ifGenerationMatch: got %q, want it unset", precondition)
	}
}
