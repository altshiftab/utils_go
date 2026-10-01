package create_folder_config

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
		wantConflictBehavior conflict_behavior.ConflictBehavior
		wantMethod           string
	}{
		{name: "defaults", wantConflictBehavior: conflict_behavior.Fail, wantMethod: fetch_config.DefaultMethod},
		{
			name:                 "set",
			options:              []Option{WithConflictBehavior(conflict_behavior.Rename), WithFetchOptions(fetch_config.WithMethod(http.MethodPut))},
			wantConflictBehavior: conflict_behavior.Rename,
			wantMethod:           http.MethodPut,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			config := New(testCase.options...)
			if config.ConflictBehavior != testCase.wantConflictBehavior {
				t.Errorf("ConflictBehavior = %q, want %q", config.ConflictBehavior, testCase.wantConflictBehavior)
			}
			if applied := fetch_config.New(config.FetchOptions...); applied.Method != testCase.wantMethod {
				t.Errorf("applied fetch Method = %q, want %q", applied.Method, testCase.wantMethod)
			}
		})
	}
}
