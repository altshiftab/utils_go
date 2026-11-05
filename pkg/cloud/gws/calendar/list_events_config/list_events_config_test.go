package list_events_config

import (
	"net/http"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if !defaults.TimeMin.IsZero() || !defaults.TimeMax.IsZero() || defaults.Query != "" || defaults.SingleEvents ||
		defaults.OrderBy != "" || defaults.MaxResults != 0 || defaults.PageToken != "" || defaults.TimeZone != "" ||
		defaults.ShowDeleted || len(defaults.FetchOptions) != 0 {
		t.Errorf("defaults = %+v, want zero", defaults)
	}

	start := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	config := New(
		WithTimeMin(start), WithTimeMax(start.Add(time.Hour)), WithQuery("q"), WithSingleEvents(true),
		WithOrderBy(OrderByStartTime), WithMaxResults(5), WithPageToken("p"), WithTimeZone("Europe/Stockholm"),
		WithShowDeleted(true), WithFetchOptions(fetch_config.WithMethod(http.MethodGet)),
	)

	testCases := []struct {
		name string
		got  any
		want any
	}{
		{name: "time min", got: config.TimeMin, want: start},
		{name: "time max", got: config.TimeMax, want: start.Add(time.Hour)},
		{name: "query", got: config.Query, want: "q"},
		{name: "single events", got: config.SingleEvents, want: true},
		{name: "order by", got: config.OrderBy, want: "startTime"},
		{name: "max results", got: config.MaxResults, want: 5},
		{name: "page token", got: config.PageToken, want: "p"},
		{name: "time zone", got: config.TimeZone, want: "Europe/Stockholm"},
		{name: "show deleted", got: config.ShowDeleted, want: true},
		{name: "fetch options", got: len(config.FetchOptions), want: 1},
	}
	for _, testCase := range testCases {
		if testCase.got != testCase.want {
			t.Errorf("%s = %v, want %v", testCase.name, testCase.got, testCase.want)
		}
	}
}
