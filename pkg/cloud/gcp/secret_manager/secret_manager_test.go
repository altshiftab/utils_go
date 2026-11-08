package secret_manager

import (
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/secret_manager/secret_manager_config"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestAccessSecretVersion(t *testing.T) {
	t.Parallel()

	secret := []byte(`{"accounts":[]}`)
	checksum := strconv.FormatUint(uint64(crc32.Checksum(secret, castagnoli)), 10)

	testCases := []struct {
		name     string
		status   int
		response any
		want     string
		wantErr  bool
	}{
		{
			name:     "with a checksum",
			status:   http.StatusOK,
			response: map[string]any{"name": "projects/1/secrets/s/versions/7", "payload": map[string]any{"data": base64.StdEncoding.EncodeToString(secret), "dataCrc32c": checksum}},
			want:     string(secret),
		},
		{
			name:     "without a checksum",
			status:   http.StatusOK,
			response: map[string]any{"name": "projects/1/secrets/s/versions/7", "payload": map[string]any{"data": base64.StdEncoding.EncodeToString(secret)}},
			want:     string(secret),
		},
		{
			name:     "a checksum that does not match",
			status:   http.StatusOK,
			response: map[string]any{"payload": map[string]any{"data": base64.StdEncoding.EncodeToString(secret), "dataCrc32c": "1"}},
			wantErr:  true,
		},
		{
			name:     "no payload",
			status:   http.StatusOK,
			response: map[string]any{"name": "x"},
			wantErr:  true,
		},
		{
			name:     "data that is not base64",
			status:   http.StatusOK,
			response: map[string]any{"payload": map[string]any{"data": "!!"}},
			wantErr:  true,
		},
		{
			name:     "a body that is not json",
			status:   http.StatusOK,
			response: "c2VjcmV0",
			wantErr:  true,
		},
		{
			name:     "refused",
			status:   http.StatusForbidden,
			response: map[string]any{"error": map[string]any{"code": 403}},
			wantErr:  true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("method = %s, want GET", r.Method)
				}
				if want := "/v1/projects/p/secrets/s/versions/latest:access"; r.URL.EscapedPath() != want {
					t.Errorf("path = %s, want %s", r.URL.EscapedPath(), want)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(testCase.status)
				if err := json.MarshalWrite(w, testCase.response); err != nil {
					t.Errorf("marshal: %v", err)
				}
			}))
			t.Cleanup(server.Close)
			u, err := url.Parse(server.URL)
			if err != nil {
				t.Fatalf("parse url: %v", err)
			}

			client := NewClient(secret_manager_config.WithBaseUrl(u), secret_manager_config.WithFetchOptions(fetch_config.WithHttpClient(server.Client())))
			got, err := client.AccessSecretVersion(context.Background(), VersionName("p", "s", "latest"))
			if (err != nil) != testCase.wantErr {
				t.Fatalf("%s: err = %v, want error %t", testCase.name, err, testCase.wantErr)
			}
			if testCase.wantErr {
				return
			}
			if string(got.Payload) != testCase.want {
				t.Errorf("%s: payload = %q, want %q", testCase.name, got.Payload, testCase.want)
			}
			if got.Name != "projects/1/secrets/s/versions/7" {
				t.Errorf("%s: name = %q", testCase.name, got.Name)
			}
		})
	}
}

func TestAccessSecretVersionEmptyName(t *testing.T) {
	t.Parallel()

	if _, err := NewClient().AccessSecretVersion(context.Background(), ""); err == nil {
		t.Error("expected an error for an empty version name")
	}
}

func TestAddAndDestroySecretVersion(t *testing.T) {
	t.Parallel()

	data := []byte(`{"accounts":[{"email":"a@example.com"}]}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.EscapedPath() {
		case "/v1/projects/p/secrets/s:addVersion":
			var body struct {
				Payload struct {
					Data       string `json:"data"`
					DataCrc32c string `json:"dataCrc32c"`
				} `json:"payload"`
			}
			if err := json.UnmarshalRead(r.Body, &body); err != nil {
				t.Errorf("unmarshal: %v", err)
			}
			decoded, err := base64.StdEncoding.DecodeString(body.Payload.Data)
			if err != nil || string(decoded) != string(data) {
				t.Errorf("payload = %q, %v", decoded, err)
			}
			if body.Payload.DataCrc32c != strconv.FormatUint(uint64(crc32.Checksum(data, castagnoli)), 10) {
				t.Errorf("crc32c = %q", body.Payload.DataCrc32c)
			}
			_, _ = w.Write([]byte(`{"name":"projects/1/secrets/s/versions/8","state":"ENABLED"}`))
		case "/v1/projects/1/secrets/s/versions/7:destroy":
			_, _ = w.Write([]byte(`{"name":"projects/1/secrets/s/versions/7","state":"DESTROYED"}`))
		default:
			t.Errorf("unexpected path %s", r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	client := NewClient(secret_manager_config.WithBaseUrl(u), secret_manager_config.WithFetchOptions(fetch_config.WithHttpClient(server.Client())))
	ctx := context.Background()

	name, err := client.AddSecretVersion(ctx, SecretName("p", "s"), data)
	if err != nil || name != "projects/1/secrets/s/versions/8" {
		t.Errorf("add = %q, %v", name, err)
	}
	if err := client.DestroySecretVersion(ctx, "projects/1/secrets/s/versions/7"); err != nil {
		t.Errorf("destroy: %v", err)
	}

	testCases := []struct {
		name string
		call func() error
	}{
		{name: "add, an empty payload", call: func() error { _, err := client.AddSecretVersion(ctx, SecretName("p", "s"), nil); return err }},
		{name: "add, no secret", call: func() error { _, err := client.AddSecretVersion(ctx, "", data); return err }},
		{name: "destroy, no version", call: func() error { return client.DestroySecretVersion(ctx, "") }},
	}
	for _, testCase := range testCases {
		if err := testCase.call(); err == nil {
			t.Errorf("%s: expected an error", testCase.name)
		}
	}
}

func TestListSecretVersions(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if want := "/v1/projects/p/secrets/s/versions"; r.URL.EscapedPath() != want {
			t.Errorf("path = %s, want %s", r.URL.EscapedPath(), want)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("pageToken") == "" {
			_, _ = w.Write([]byte(`{"versions":[{"name":"projects/1/secrets/s/versions/3","state":"ENABLED"}],"nextPageToken":"n"}`))
			return
		}
		_, _ = w.Write([]byte(`{"versions":[{"name":"projects/1/secrets/s/versions/2","state":"DESTROYED"}]}`))
	}))
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	versions, err := NewClient(secret_manager_config.WithBaseUrl(u), secret_manager_config.WithFetchOptions(fetch_config.WithHttpClient(server.Client()))).ListSecretVersions(context.Background(), SecretName("p", "s"))
	if err != nil || len(versions) != 2 || versions[0].State != StateEnabled || versions[1].Name != "projects/1/secrets/s/versions/2" {
		t.Errorf("versions = %+v, %v", versions, err)
	}
	if _, err := NewClient().ListSecretVersions(context.Background(), ""); err == nil {
		t.Error("expected an error for an empty secret name")
	}
}
