package list_drafts_config

import (
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.Query != "" {
		t.Errorf("default Query = %q, want empty", defaults.Query)
	}
	if defaults.IncludeSpamTrash {
		t.Error("default IncludeSpamTrash = true, want false")
	}
	if defaults.MaxResults != 0 {
		t.Errorf("default MaxResults = %d, want 0", defaults.MaxResults)
	}
	if defaults.PageToken != "" {
		t.Errorf("default PageToken = %q, want empty", defaults.PageToken)
	}
	if len(defaults.FetchOptions) != 0 {
		t.Errorf("default FetchOptions len = %d, want 0", len(defaults.FetchOptions))
	}

	config := New(
		WithQuery("to:someone@example.com"),
		WithIncludeSpamTrash(true),
		WithMaxResults(50),
		WithPageToken("token-abc"),
		WithFetchOptions(fetch_config.WithMethod(http.MethodGet)),
	)

	if config.Query != "to:someone@example.com" {
		t.Errorf("Query = %q", config.Query)
	}
	if !config.IncludeSpamTrash {
		t.Error("IncludeSpamTrash = false, want true")
	}
	if config.MaxResults != 50 {
		t.Errorf("MaxResults = %d, want 50", config.MaxResults)
	}
	if config.PageToken != "token-abc" {
		t.Errorf("PageToken = %q, want token-abc", config.PageToken)
	}
	if len(config.FetchOptions) != 1 {
		t.Errorf("FetchOptions len = %d, want 1", len(config.FetchOptions))
	}
}
