package mail_config

import (
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	baseUrl := &url.URL{Scheme: "https", Host: "graph.example.com"}
	testCases := []struct {
		name             string
		options          []Option
		wantBaseUrl      *url.URL
		wantFetchOptions int
	}{
		{name: "no options"},
		{name: "a base url", options: []Option{WithBaseUrl(baseUrl)}, wantBaseUrl: baseUrl},
		{
			name:             "fetch options add up",
			options:          []Option{WithFetchOptions(fetch_config.WithMethod("GET")), WithFetchOptions(fetch_config.WithBody(nil), fetch_config.WithBody(nil))},
			wantFetchOptions: 3,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			config := New(testCase.options...)
			if config.BaseUrl != testCase.wantBaseUrl || len(config.FetchOptions) != testCase.wantFetchOptions {
				t.Errorf("%s: config %+v", testCase.name, config)
			}
		})
	}
}
