package list_comments_config

import (
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.PageSize != 0 || defaults.PageToken != "" || defaults.IncludeDeleted || defaults.Fields != "" ||
		len(defaults.FetchOptions) != 0 {
		t.Errorf("defaults = %+v, want zero", defaults)
	}

	config := New(
		WithPageSize(100),
		WithPageToken("p1"),
		WithIncludeDeleted(true),
		WithFields("comments(id)"),
		WithFetchOptions(fetch_config.WithMethod(http.MethodGet)),
	)
	if config.PageSize != 100 || config.PageToken != "p1" || !config.IncludeDeleted || config.Fields != "comments(id)" ||
		len(config.FetchOptions) != 1 {
		t.Errorf("config = %+v", config)
	}
}
