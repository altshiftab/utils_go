package artifact_registry_config

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
}

func TestWithBaseUrl(t *testing.T) {
	t.Parallel()

	baseUrl, err := url.Parse("https://artifactregistry.googleapis.com")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}

	config := New(WithBaseUrl(baseUrl))
	if config.BaseUrl == nil {
		t.Fatal("BaseUrl is nil")
	}
	if config.BaseUrl.String() != baseUrl.String() {
		t.Errorf("BaseUrl = %q, want %q", config.BaseUrl.String(), baseUrl.String())
	}
}

func TestWithFetchOptions(t *testing.T) {
	t.Parallel()

	config := New(
		WithFetchOptions(fetch_config.WithMethod(http.MethodPost)),
		WithFetchOptions(fetch_config.WithSkipErrorOnStatus(true)),
	)
	if len(config.FetchOptions) != 2 {
		t.Fatalf("FetchOptions len = %d, want 2 (cumulative append)", len(config.FetchOptions))
	}

	fetchConfig := fetch_config.New(config.FetchOptions...)
	if fetchConfig.Method != http.MethodPost {
		t.Errorf("applied Method = %q, want %q", fetchConfig.Method, http.MethodPost)
	}
	if !fetchConfig.SkipErrorOnStatus {
		t.Error("applied SkipErrorOnStatus = false, want true")
	}
}
