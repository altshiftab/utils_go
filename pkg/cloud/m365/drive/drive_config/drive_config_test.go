package drive_config

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.BaseUrl != nil || len(defaults.FetchOptions) != 0 || defaults.UploadHttpClient != nil || defaults.UploadChunkSize != 0 {
		t.Errorf("defaults = %+v, want zero", defaults)
	}

	baseUrl := &url.URL{Scheme: "https", Host: "graph.example"}
	uploadHttpClient := &http.Client{}
	config := New(
		WithBaseUrl(baseUrl),
		WithFetchOptions(fetch_config.WithMethod(http.MethodPost)),
		WithUploadHttpClient(uploadHttpClient),
		WithUploadChunkSize(640<<10),
	)
	if config.BaseUrl != baseUrl {
		t.Errorf("BaseUrl = %v, want %v", config.BaseUrl, baseUrl)
	}
	if applied := fetch_config.New(config.FetchOptions...); applied.Method != http.MethodPost {
		t.Errorf("applied fetch Method = %q, want %q", applied.Method, http.MethodPost)
	}
	if config.UploadHttpClient != uploadHttpClient {
		t.Error("UploadHttpClient not set")
	}
	if config.UploadChunkSize != 640<<10 {
		t.Errorf("UploadChunkSize = %d", config.UploadChunkSize)
	}
}
