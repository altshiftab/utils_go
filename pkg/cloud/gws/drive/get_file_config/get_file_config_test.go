package get_file_config

import (
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.Fields != "" || len(defaults.FetchOptions) != 0 {
		t.Errorf("defaults = %+v, want zero", defaults)
	}

	config := New(WithFields("id, name"), WithFetchOptions(fetch_config.WithMethod(http.MethodGet)))
	if config.Fields != "id, name" {
		t.Errorf("Fields = %q", config.Fields)
	}
	if len(config.FetchOptions) != 1 {
		t.Errorf("FetchOptions len = %d, want 1", len(config.FetchOptions))
	}
}
