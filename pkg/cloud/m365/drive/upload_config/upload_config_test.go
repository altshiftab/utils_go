package upload_config

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
		wantContentType      string
		wantMethod           string
	}{
		{name: "defaults", wantConflictBehavior: conflict_behavior.Fail, wantMethod: fetch_config.DefaultMethod},
		{
			name: "set",
			options: []Option{
				WithConflictBehavior(conflict_behavior.Replace),
				WithContentType("text/csv"),
				WithFetchOptions(fetch_config.WithMethod(http.MethodPost)),
			},
			wantConflictBehavior: conflict_behavior.Replace,
			wantContentType:      "text/csv",
			wantMethod:           http.MethodPost,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			config := New(testCase.options...)
			if config.ConflictBehavior != testCase.wantConflictBehavior {
				t.Errorf("ConflictBehavior = %q, want %q", config.ConflictBehavior, testCase.wantConflictBehavior)
			}
			if config.ContentType != testCase.wantContentType {
				t.Errorf("ContentType = %q, want %q", config.ContentType, testCase.wantContentType)
			}
			if applied := fetch_config.New(config.FetchOptions...); applied.Method != testCase.wantMethod {
				t.Errorf("applied fetch Method = %q, want %q", applied.Method, testCase.wantMethod)
			}
		})
	}
}
