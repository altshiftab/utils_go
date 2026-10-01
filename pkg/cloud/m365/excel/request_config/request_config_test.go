package request_config

import (
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.SessionId != "" || len(defaults.FetchOptions) != 0 {
		t.Errorf("defaults = %+v, want zero", defaults)
	}

	config := New(WithSessionId("sess-1"), WithFetchOptions(fetch_config.WithMethod(http.MethodPost)))
	if config.SessionId != "sess-1" {
		t.Errorf("SessionId = %q", config.SessionId)
	}
	if applied := fetch_config.New(config.FetchOptions...); applied.Method != http.MethodPost {
		t.Errorf("applied fetch Method = %q, want %q", applied.Method, http.MethodPost)
	}
}
