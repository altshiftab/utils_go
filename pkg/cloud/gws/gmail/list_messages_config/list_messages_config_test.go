package list_messages_config

import (
	"net/http"
	"slices"
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
	if len(defaults.LabelIds) != 0 {
		t.Errorf("default LabelIds len = %d, want 0", len(defaults.LabelIds))
	}
	if len(defaults.FetchOptions) != 0 {
		t.Errorf("default FetchOptions len = %d, want 0", len(defaults.FetchOptions))
	}

	config := New(
		WithQuery("after:1756382400"),
		WithIncludeSpamTrash(true),
		WithMaxResults(500),
		WithLabelIds("INBOX"),
		WithLabelIds("UNREAD"),
		WithFetchOptions(fetch_config.WithMethod(http.MethodGet)),
	)

	if config.Query != "after:1756382400" {
		t.Errorf("Query = %q", config.Query)
	}
	if !config.IncludeSpamTrash {
		t.Error("IncludeSpamTrash = false, want true")
	}
	if config.MaxResults != 500 {
		t.Errorf("MaxResults = %d, want 500", config.MaxResults)
	}
	// Repeats accumulate rather than replace, as elsewhere.
	if !slices.Equal(config.LabelIds, []string{"INBOX", "UNREAD"}) {
		t.Errorf("LabelIds = %v, want [INBOX UNREAD]", config.LabelIds)
	}
	if len(config.FetchOptions) != 1 {
		t.Errorf("FetchOptions len = %d, want 1", len(config.FetchOptions))
	}
}
