package list_files_config

import (
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.Query != "" || defaults.PageSize != 0 || defaults.PageToken != "" || defaults.OrderBy != "" ||
		defaults.Fields != "" || defaults.Corpora != "" || defaults.IncludeItemsFromAllDrives || len(defaults.FetchOptions) != 0 {
		t.Errorf("defaults = %+v, want zero", defaults)
	}

	config := New(
		WithQuery("trashed = false"),
		WithPageSize(50),
		WithPageToken("p"),
		WithOrderBy("name"),
		WithFields("files(id)"),
		WithCorpora(CorporaAllDrives),
		WithIncludeItemsFromAllDrives(true),
		WithFetchOptions(fetch_config.WithMethod(http.MethodGet)),
	)

	testCases := []struct {
		name string
		got  any
		want any
	}{
		{name: "query", got: config.Query, want: "trashed = false"},
		{name: "page size", got: config.PageSize, want: 50},
		{name: "page token", got: config.PageToken, want: "p"},
		{name: "order by", got: config.OrderBy, want: "name"},
		{name: "fields", got: config.Fields, want: "files(id)"},
		{name: "corpora", got: config.Corpora, want: "allDrives"},
		{name: "include items from all drives", got: config.IncludeItemsFromAllDrives, want: true},
		{name: "fetch options", got: len(config.FetchOptions), want: 1},
	}
	for _, testCase := range testCases {
		if testCase.got != testCase.want {
			t.Errorf("%s = %v, want %v", testCase.name, testCase.got, testCase.want)
		}
	}
}
