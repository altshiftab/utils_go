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
)

func TestAccessSecretVersion(t *testing.T) {
	t.Parallel()

	secret := []byte(`{"accounts":[]}`)
	checksum := strconv.FormatUint(uint64(crc32.Checksum(secret, castagnoli)), 10)

	testCases := []struct {
		name     string
		status   int
		response map[string]any
		want     string
		wantErr  bool
	}{
		{
			name:     "with a checksum",
			status:   http.StatusOK,
			response: map[string]any{"payload": map[string]any{"data": base64.StdEncoding.EncodeToString(secret), "dataCrc32c": checksum}},
			want:     string(secret),
		},
		{
			name:     "without a checksum",
			status:   http.StatusOK,
			response: map[string]any{"payload": map[string]any{"data": base64.StdEncoding.EncodeToString(secret)}},
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

			client := NewClient(secret_manager_config.WithBaseUrl(u))
			got, err := client.AccessSecretVersion(context.Background(), VersionName("p", "s", "latest"))
			if (err != nil) != testCase.wantErr {
				t.Fatalf("%s: err = %v, want error %t", testCase.name, err, testCase.wantErr)
			}
			if string(got) != testCase.want {
				t.Errorf("%s: payload = %q, want %q", testCase.name, got, testCase.want)
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
