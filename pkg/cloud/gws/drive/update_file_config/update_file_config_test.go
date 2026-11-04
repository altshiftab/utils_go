package update_file_config

import (
	"net/http"
	"slices"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.Fields != "" || len(defaults.AddParents) != 0 || len(defaults.RemoveParents) != 0 || len(defaults.FetchOptions) != 0 {
		t.Errorf("defaults = %+v, want zero", defaults)
	}

	config := New(
		WithFields("id, parents"),
		WithAddParents("a"),
		WithAddParents("b"),
		WithRemoveParents("c"),
		WithFetchOptions(fetch_config.WithMethod(http.MethodPatch)),
	)
	if config.Fields != "id, parents" {
		t.Errorf("Fields = %q", config.Fields)
	}
	// Repeats accumulate rather than replace, as elsewhere.
	if !slices.Equal(config.AddParents, []string{"a", "b"}) {
		t.Errorf("AddParents = %v, want [a b]", config.AddParents)
	}
	if !slices.Equal(config.RemoveParents, []string{"c"}) {
		t.Errorf("RemoveParents = %v, want [c]", config.RemoveParents)
	}
	if len(config.FetchOptions) != 1 {
		t.Errorf("FetchOptions len = %d, want 1", len(config.FetchOptions))
	}
}
