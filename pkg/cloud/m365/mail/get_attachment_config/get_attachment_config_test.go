package get_attachment_config

import (
	"slices"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	config := New(
		WithSelect("id"),
		WithSelect("name", "size"),
		WithExpand(ExpandItem),
		WithFetchOptions(fetch_config.WithMethod("GET")),
	)

	testCases := []struct {
		name string
		ok   bool
	}{
		{name: "select adds up", ok: slices.Equal(config.Select, []string{"id", "name", "size"})},
		{name: "expand", ok: config.Expand == "microsoft.graph.itemattachment/item"},
		{name: "fetch options", ok: len(config.FetchOptions) == 1},
	}
	for _, testCase := range testCases {
		if !testCase.ok {
			t.Errorf("%s: config %+v", testCase.name, config)
		}
	}

	if empty := New(); empty.Select != nil || empty.Expand != "" {
		t.Errorf("no options: config %+v", empty)
	}
}
