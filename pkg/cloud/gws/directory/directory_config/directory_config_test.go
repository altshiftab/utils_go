package directory_config

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.BaseUrl != nil {
		t.Errorf("default BaseUrl = %v, want nil", defaults.BaseUrl)
	}
	if len(defaults.FetchOptions) != 0 {
		t.Errorf("default FetchOptions len = %d, want 0", len(defaults.FetchOptions))
	}

	baseUrl := &url.URL{Scheme: "https", Host: "directory.example.com"}
	config := New(
		WithBaseUrl(baseUrl),
		WithFetchOptions(fetch_config.WithMethod(http.MethodPost)),
	)
	if config.BaseUrl != baseUrl {
		t.Errorf("BaseUrl = %v, want %v", config.BaseUrl, baseUrl)
	}
	if len(config.FetchOptions) != 1 {
		t.Fatalf("FetchOptions len = %d, want 1", len(config.FetchOptions))
	}
	if applied := fetch_config.New(config.FetchOptions...); applied.Method != http.MethodPost {
		t.Errorf("applied fetch Method = %q, want %q", applied.Method, http.MethodPost)
	}
}
