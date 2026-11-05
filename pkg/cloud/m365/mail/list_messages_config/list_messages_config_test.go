package list_messages_config

import (
	"slices"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	config := New(
		WithFolderId("inbox"),
		WithSearch("budget"),
		WithFilter("isRead eq false"),
		WithOrderBy("receivedDateTime desc"),
		WithOrderBy("subject"),
		WithSelect("id"),
		WithSelect("subject", "from"),
		WithExpand("attachments"),
		WithTop(25),
		WithBodyContentType("text"),
		WithNextLink("https://graph.microsoft.com/v1.0/users/x/messages?$skip=10"),
		WithFetchOptions(fetch_config.WithMethod("GET")),
	)

	testCases := []struct {
		name string
		ok   bool
	}{
		{name: "folder", ok: config.FolderId == "inbox"},
		{name: "search", ok: config.Search == "budget"},
		{name: "filter", ok: config.Filter == "isRead eq false"},
		{name: "order by adds up", ok: slices.Equal(config.OrderBy, []string{"receivedDateTime desc", "subject"})},
		{name: "select adds up", ok: slices.Equal(config.Select, []string{"id", "subject", "from"})},
		{name: "expand", ok: config.Expand == "attachments"},
		{name: "top", ok: config.Top == 25},
		{name: "body content type", ok: config.BodyContentType == "text"},
		{name: "next link", ok: config.NextLink == "https://graph.microsoft.com/v1.0/users/x/messages?$skip=10"},
		{name: "fetch options", ok: len(config.FetchOptions) == 1},
	}
	for _, testCase := range testCases {
		if !testCase.ok {
			t.Errorf("%s: config %+v", testCase.name, config)
		}
	}

	if empty := New(); empty.FolderId != "" || empty.Top != 0 || empty.Select != nil || empty.NextLink != "" {
		t.Errorf("no options: config %+v", empty)
	}
}
