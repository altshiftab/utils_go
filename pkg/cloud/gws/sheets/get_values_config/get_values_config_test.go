package get_values_config

import (
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.MajorDimension != "" {
		t.Errorf("default MajorDimension = %q, want empty", defaults.MajorDimension)
	}
	if len(defaults.FetchOptions) != 0 {
		t.Errorf("default FetchOptions len = %d, want 0", len(defaults.FetchOptions))
	}

	config := New(
		WithMajorDimension(MajorDimensionColumns),
		WithFetchOptions(fetch_config.WithMethod(http.MethodPost)),
	)
	if config.MajorDimension != MajorDimensionColumns {
		t.Errorf("MajorDimension = %q, want %q", config.MajorDimension, MajorDimensionColumns)
	}
	if len(config.FetchOptions) != 1 {
		t.Fatalf("FetchOptions len = %d, want 1", len(config.FetchOptions))
	}
	if applied := fetch_config.New(config.FetchOptions...); applied.Method != http.MethodPost {
		t.Errorf("applied fetch Method = %q, want %q", applied.Method, http.MethodPost)
	}
}
