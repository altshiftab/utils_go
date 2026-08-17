package get_spreadsheet_config

import (
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.Fields != "" {
		t.Errorf("default Fields = %q, want empty", defaults.Fields)
	}
	if len(defaults.FetchOptions) != 0 {
		t.Errorf("default FetchOptions len = %d, want 0", len(defaults.FetchOptions))
	}

	config := New(
		WithFields("sheets.properties(sheetId,title)"),
		WithFetchOptions(fetch_config.WithMethod(http.MethodPost)),
	)
	if config.Fields != "sheets.properties(sheetId,title)" {
		t.Errorf("Fields = %q, want %q", config.Fields, "sheets.properties(sheetId,title)")
	}
	if len(config.FetchOptions) != 1 {
		t.Fatalf("FetchOptions len = %d, want 1", len(config.FetchOptions))
	}
	if applied := fetch_config.New(config.FetchOptions...); applied.Method != http.MethodPost {
		t.Errorf("applied fetch Method = %q, want %q", applied.Method, http.MethodPost)
	}
}
