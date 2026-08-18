package github_config

import (
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("nothing is set by default", func(t *testing.T) {
		t.Parallel()

		config := New()
		if config.BaseUrl != nil {
			t.Errorf("BaseUrl = %v, want nil", config.BaseUrl)
		}
		if config.ArchiveBaseUrl != nil {
			t.Errorf("ArchiveBaseUrl = %v, want nil", config.ArchiveBaseUrl)
		}
		if len(config.FetchOptions) != 0 {
			t.Errorf("FetchOptions = %v, want none", config.FetchOptions)
		}
	})

	t.Run("a nil option is ignored", func(t *testing.T) {
		t.Parallel()

		if config := New(nil); config == nil {
			t.Fatal("expected a config, got nil")
		}
	})
}

func TestOptions(t *testing.T) {
	t.Parallel()

	baseUrl := &url.URL{Scheme: "https", Host: "example.com"}

	testCases := []struct {
		name   string
		option Option
		check  func(t *testing.T, config *Config)
	}{
		{
			name:   "base url is applied",
			option: WithBaseUrl(baseUrl),
			check: func(t *testing.T, config *Config) {
				if config.BaseUrl != baseUrl {
					t.Errorf("BaseUrl = %v", config.BaseUrl)
				}
			},
		},
		{
			name:   "archive base url is applied",
			option: WithArchiveBaseUrl(baseUrl),
			check: func(t *testing.T, config *Config) {
				if config.ArchiveBaseUrl != baseUrl {
					t.Errorf("ArchiveBaseUrl = %v", config.ArchiveBaseUrl)
				}
			},
		},
		{
			name:   "fetch options are applied",
			option: WithFetchOptions(fetch_config.WithMethod("HEAD")),
			check: func(t *testing.T, config *Config) {
				if len(config.FetchOptions) != 1 {
					t.Errorf("FetchOptions = %v, want one", config.FetchOptions)
				}
			},
		},
		{
			name:   "a token becomes a fetch option",
			option: WithToken("secret"),
			check: func(t *testing.T, config *Config) {
				if len(config.FetchOptions) != 1 {
					t.Errorf("FetchOptions = %v, want one", config.FetchOptions)
				}
			},
		},
		{
			// An empty token must not add an Authorization header saying
			// "Bearer ", which reads as a malformed credential rather than none.
			name:   "an empty token adds nothing",
			option: WithToken(""),
			check: func(t *testing.T, config *Config) {
				if len(config.FetchOptions) != 0 {
					t.Errorf("FetchOptions = %v, want none", config.FetchOptions)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			config := New(testCase.option)
			testCase.check(t, config)
		})
	}
}
