package get_message_config

import (
	"slices"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	config := New(
		WithSelect("subject"),
		WithSelect("body", "categories"),
		WithExpand("attachments"),
		WithBodyContentType("html"),
		WithFetchOptions(fetch_config.WithMethod("GET")),
	)

	testCases := []struct {
		name string
		ok   bool
	}{
		{name: "select adds up", ok: slices.Equal(config.Select, []string{"subject", "body", "categories"})},
		{name: "expand", ok: config.Expand == "attachments"},
		{name: "body content type", ok: config.BodyContentType == "html"},
		{name: "fetch options", ok: len(config.FetchOptions) == 1},
	}
	for _, testCase := range testCases {
		if !testCase.ok {
			t.Errorf("%s: config %+v", testCase.name, config)
		}
	}

	if empty := New(); empty.Select != nil || empty.Expand != "" || empty.BodyContentType != "" {
		t.Errorf("no options: config %+v", empty)
	}
}
