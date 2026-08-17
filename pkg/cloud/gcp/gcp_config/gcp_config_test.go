package gcp_config

import (
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if len(defaults.FetchOptions) != 0 {
		t.Errorf("default FetchOptions len = %d, want 0", len(defaults.FetchOptions))
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
