package blob_storage_config

import (
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	baseUrl := &url.URL{Scheme: "http", Host: "127.0.0.1:10000"}

	testCases := []struct {
		name             string
		options          []Option
		wantBaseUrl      *url.URL
		wantFetchOptions int
	}{
		{name: "no options"},
		{name: "a nil option", options: []Option{nil}},
		{name: "a base url", options: []Option{WithBaseUrl(baseUrl)}, wantBaseUrl: baseUrl},
		{
			name:             "fetch options accumulate",
			options:          []Option{WithFetchOptions(fetch_config.WithMethod("GET")), WithFetchOptions(fetch_config.WithMethod("PUT"), nil)},
			wantFetchOptions: 3,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			config := New(testCase.options...)
			if config.BaseUrl != testCase.wantBaseUrl {
				t.Errorf("base url %v, want %v", config.BaseUrl, testCase.wantBaseUrl)
			}
			if len(config.FetchOptions) != testCase.wantFetchOptions {
				t.Errorf("%d fetch options, want %d", len(config.FetchOptions), testCase.wantFetchOptions)
			}
		})
	}
}
