package reader_config

import (
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.Generation != 0 || defaults.Offset != 0 || defaults.Length != 0 || len(defaults.FetchOptions) != 0 {
		t.Errorf("defaults = %+v, want zero apart from the reconnect limit", defaults)
	}
	if defaults.MaxReconnects != DefaultMaxReconnects {
		t.Errorf("MaxReconnects = %d, want %d", defaults.MaxReconnects, DefaultMaxReconnects)
	}

	config := New(
		WithGeneration(42),
		WithRange(100, 200),
		WithMaxReconnects(1),
		WithFetchOptions(fetch_config.WithMethod(http.MethodGet)),
		nil,
	)
	if config.Generation != 42 {
		t.Errorf("Generation = %d", config.Generation)
	}
	if config.Offset != 100 || config.Length != 200 {
		t.Errorf("range = %d, %d", config.Offset, config.Length)
	}
	if config.MaxReconnects != 1 {
		t.Errorf("MaxReconnects = %d", config.MaxReconnects)
	}
	if len(config.FetchOptions) != 1 {
		t.Errorf("FetchOptions len = %d, want 1", len(config.FetchOptions))
	}
}
