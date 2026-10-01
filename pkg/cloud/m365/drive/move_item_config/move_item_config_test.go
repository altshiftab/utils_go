package move_item_config

import (
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/types/conflict_behavior"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                 string
		options              []Option
		wantName             string
		wantConflictBehavior conflict_behavior.ConflictBehavior
		wantMethod           string
	}{
		{name: "defaults leave the API default", wantMethod: fetch_config.DefaultMethod},
		{
			name: "set",
			options: []Option{
				WithName("ny.xlsx"),
				WithConflictBehavior(conflict_behavior.Replace),
				WithFetchOptions(fetch_config.WithMethod(http.MethodPatch)),
			},
			wantName:             "ny.xlsx",
			wantConflictBehavior: conflict_behavior.Replace,
			wantMethod:           http.MethodPatch,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			config := New(testCase.options...)
			if config.Name != testCase.wantName {
				t.Errorf("Name = %q, want %q", config.Name, testCase.wantName)
			}
			if config.ConflictBehavior != testCase.wantConflictBehavior {
				t.Errorf("ConflictBehavior = %q, want %q", config.ConflictBehavior, testCase.wantConflictBehavior)
			}
			if applied := fetch_config.New(config.FetchOptions...); applied.Method != testCase.wantMethod {
				t.Errorf("applied fetch Method = %q, want %q", applied.Method, testCase.wantMethod)
			}
		})
	}
}
