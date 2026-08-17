package list_history_config

import (
	"net/http"
	"slices"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if len(defaults.HistoryTypes) != 0 {
		t.Errorf("default HistoryTypes len = %d, want 0", len(defaults.HistoryTypes))
	}
	if defaults.LabelId != "" {
		t.Errorf("default LabelId = %q, want empty", defaults.LabelId)
	}
	if len(defaults.FetchOptions) != 0 {
		t.Errorf("default FetchOptions len = %d, want 0", len(defaults.FetchOptions))
	}

	config := New(
		WithHistoryTypes(HistoryTypeMessageAdded, HistoryTypeLabelRemoved),
		WithLabelId("INBOX"),
		WithFetchOptions(fetch_config.WithMethod(http.MethodPost)),
	)
	if !slices.Equal(config.HistoryTypes, []HistoryType{HistoryTypeMessageAdded, HistoryTypeLabelRemoved}) {
		t.Errorf("HistoryTypes = %#v", config.HistoryTypes)
	}
	if config.LabelId != "INBOX" {
		t.Errorf("LabelId = %q, want %q", config.LabelId, "INBOX")
	}
	if len(config.FetchOptions) != 1 {
		t.Fatalf("FetchOptions len = %d, want 1", len(config.FetchOptions))
	}
	if applied := fetch_config.New(config.FetchOptions...); applied.Method != http.MethodPost {
		t.Errorf("applied fetch Method = %q, want %q", applied.Method, http.MethodPost)
	}
}

func TestWithHistoryTypesAppends(t *testing.T) {
	t.Parallel()

	config := New(
		WithHistoryTypes(HistoryTypeMessageAdded),
		WithHistoryTypes(HistoryTypeMessageDeleted, HistoryTypeLabelAdded),
	)
	want := []HistoryType{HistoryTypeMessageAdded, HistoryTypeMessageDeleted, HistoryTypeLabelAdded}
	if !slices.Equal(config.HistoryTypes, want) {
		t.Errorf("HistoryTypes = %#v, want %#v", config.HistoryTypes, want)
	}
}

func TestHistoryTypeConstants(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		historyType HistoryType
		want        string
	}{
		{name: "messageAdded", historyType: HistoryTypeMessageAdded, want: "messageAdded"},
		{name: "messageDeleted", historyType: HistoryTypeMessageDeleted, want: "messageDeleted"},
		{name: "labelAdded", historyType: HistoryTypeLabelAdded, want: "labelAdded"},
		{name: "labelRemoved", historyType: HistoryTypeLabelRemoved, want: "labelRemoved"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if string(testCase.historyType) != testCase.want {
				t.Errorf("HistoryType = %q, want %q", testCase.historyType, testCase.want)
			}
		})
	}
}
