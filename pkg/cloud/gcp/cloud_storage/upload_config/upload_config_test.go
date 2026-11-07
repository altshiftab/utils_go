package upload_config

import (
	"net/http"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.IfAbsent || defaults.SessionUri != "" || defaults.OnSession != nil || len(defaults.FetchOptions) != 0 {
		t.Errorf("defaults = %+v, want zero apart from the limits", defaults)
	}
	if defaults.ChunkSize != DefaultChunkSize || defaults.ChunkSize%ChunkGranularity != 0 {
		t.Errorf("ChunkSize = %d, want %d", defaults.ChunkSize, DefaultChunkSize)
	}
	if defaults.MaxAttempts != DefaultMaxAttempts {
		t.Errorf("MaxAttempts = %d, want %d", defaults.MaxAttempts, DefaultMaxAttempts)
	}
	if defaults.RetryBaseDelay != DefaultRetryBaseDelay || defaults.RetryMaxDelay != DefaultRetryMaxDelay {
		t.Errorf("retry delays = %v, %v", defaults.RetryBaseDelay, defaults.RetryMaxDelay)
	}

	var told string
	config := New(
		WithIfAbsent(true),
		WithChunkSize(2*ChunkGranularity),
		WithSessionUri("https://example.com/session"),
		WithOnSession(func(uri string) { told = uri }),
		WithMaxAttempts(3),
		WithRetryDelays(time.Millisecond, time.Second),
		WithFetchOptions(fetch_config.WithMethod(http.MethodPut)),
		WithFetchOptions(fetch_config.WithMethod(http.MethodPost)),
		nil,
	)
	if !config.IfAbsent {
		t.Errorf("IfAbsent = false")
	}
	if config.ChunkSize != 2*ChunkGranularity {
		t.Errorf("ChunkSize = %d", config.ChunkSize)
	}
	if config.SessionUri != "https://example.com/session" {
		t.Errorf("SessionUri = %q", config.SessionUri)
	}
	config.OnSession("uri")
	if told != "uri" {
		t.Errorf("OnSession was not the function given")
	}
	if config.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d", config.MaxAttempts)
	}
	if config.RetryBaseDelay != time.Millisecond || config.RetryMaxDelay != time.Second {
		t.Errorf("retry delays = %v, %v", config.RetryBaseDelay, config.RetryMaxDelay)
	}
	// Repeats accumulate rather than replace, as elsewhere.
	if len(config.FetchOptions) != 2 {
		t.Errorf("FetchOptions len = %d, want 2", len(config.FetchOptions))
	}
}
