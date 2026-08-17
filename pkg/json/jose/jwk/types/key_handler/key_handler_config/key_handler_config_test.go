package key_handler_config

import (
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.FetchOptions != nil {
		t.Errorf("default FetchOptions = %v, want nil", defaults.FetchOptions)
	}

	// A single WithFetchOptions with one option stores exactly that option, and it
	// takes effect when applied to a fetch config.
	single := New(WithFetchOptions(fetch_config.WithMethod(http.MethodPost)))
	if len(single.FetchOptions) != 1 {
		t.Fatalf("FetchOptions len = %d, want 1", len(single.FetchOptions))
	}
	if fc := fetch_config.New(single.FetchOptions...); fc.Method != http.MethodPost {
		t.Errorf("method = %q, want %q", fc.Method, http.MethodPost)
	}

	// Multiple options within a single WithFetchOptions call are all appended.
	multiInOne := New(WithFetchOptions(
		fetch_config.WithMethod(http.MethodPut),
		fetch_config.WithSkipErrorOnStatus(true),
	))
	if len(multiInOne.FetchOptions) != 2 {
		t.Fatalf("FetchOptions len = %d, want 2", len(multiInOne.FetchOptions))
	}

	// Multiple WithFetchOptions calls append cumulatively.
	multiCalls := New(
		WithFetchOptions(fetch_config.WithMethod(http.MethodPatch)),
		WithFetchOptions(fetch_config.WithSkipErrorOnStatus(true)),
	)
	if len(multiCalls.FetchOptions) != 2 {
		t.Fatalf("FetchOptions len = %d, want 2", len(multiCalls.FetchOptions))
	}
	fc := fetch_config.New(multiCalls.FetchOptions...)
	if fc.Method != http.MethodPatch {
		t.Errorf("method = %q, want %q", fc.Method, http.MethodPatch)
	}
	if !fc.SkipErrorOnStatus {
		t.Errorf("SkipErrorOnStatus = false, want true")
	}
}
