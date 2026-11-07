package import_message_config

import (
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.InternalDateSource != "" {
		t.Errorf("default InternalDateSource = %q, want empty", defaults.InternalDateSource)
	}
	if defaults.NeverMarkSpam || defaults.ProcessForCalendar || defaults.Deleted {
		t.Errorf("default flags = %+v, want all false", defaults)
	}
	if len(defaults.FetchOptions) != 0 {
		t.Errorf("default FetchOptions len = %d, want 0", len(defaults.FetchOptions))
	}

	config := New(
		WithInternalDateSource(InternalDateSourceDateHeader),
		WithNeverMarkSpam(true),
		WithProcessForCalendar(true),
		WithDeleted(true),
		WithFetchOptions(fetch_config.WithMethod(http.MethodPost)),
	)
	if config.InternalDateSource != InternalDateSourceDateHeader {
		t.Errorf("InternalDateSource = %q, want %q", config.InternalDateSource, InternalDateSourceDateHeader)
	}
	if !config.NeverMarkSpam || !config.ProcessForCalendar || !config.Deleted {
		t.Errorf("flags = %+v, want all true", config)
	}
	if len(config.FetchOptions) != 1 {
		t.Fatalf("FetchOptions len = %d, want 1", len(config.FetchOptions))
	}
	if applied := fetch_config.New(config.FetchOptions...); applied.Method != http.MethodPost {
		t.Errorf("applied fetch Method = %q, want %q", applied.Method, http.MethodPost)
	}
}

func TestInternalDateSourceConstants(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source InternalDateSource
		want   string
	}{
		{name: "received time", source: InternalDateSourceReceivedTime, want: "receivedTime"},
		{name: "date header", source: InternalDateSourceDateHeader, want: "dateHeader"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if string(testCase.source) != testCase.want {
				t.Errorf("InternalDateSource = %q, want %q", testCase.source, testCase.want)
			}
		})
	}
}
