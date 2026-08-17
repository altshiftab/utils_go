package fetch_config

import (
	"bytes"
	"maps"
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config/retry_config"
)

func TestNewDefaults(t *testing.T) {
	t.Parallel()

	config := New()

	if config.Method != DefaultMethod {
		t.Errorf("Method = %q, want %q", config.Method, DefaultMethod)
	}
	if config.HttpClient != http.DefaultClient {
		t.Errorf("HttpClient = %p, want %p", config.HttpClient, http.DefaultClient)
	}
	if config.Headers != nil {
		t.Errorf("Headers = %v, want nil", config.Headers)
	}
	if config.Body != nil {
		t.Errorf("Body = %v, want nil", config.Body)
	}
	if config.SkipReadResponseBody {
		t.Error("SkipReadResponseBody = true, want false")
	}
	if config.SkipErrorOnStatus {
		t.Error("SkipErrorOnStatus = true, want false")
	}
	if config.RetryConfig != nil {
		t.Errorf("RetryConfig = %v, want nil", config.RetryConfig)
	}
}

func TestNewNilOptionSkipped(t *testing.T) {
	t.Parallel()

	config := New(nil, WithMethod(http.MethodPost), nil)
	if config.Method != http.MethodPost {
		t.Errorf("Method = %q, want %q", config.Method, http.MethodPost)
	}
}

func TestNewOptions(t *testing.T) {
	t.Parallel()

	headers := map[string]string{"X-Test": "value"}
	body := []byte("payload")
	retryConfig := retry_config.New()
	httpClient := &http.Client{}

	config := New(
		WithMethod(http.MethodPost),
		WithHeaders(headers),
		WithBody(body),
		WithSkipReadResponseBody(true),
		WithSkipErrorOnStatus(true),
		WithRetryConfig(retryConfig),
		WithHttpClient(httpClient),
	)

	if config.Method != http.MethodPost {
		t.Errorf("Method = %q, want %q", config.Method, http.MethodPost)
	}
	if config.Headers["X-Test"] != "value" {
		t.Errorf("Headers = %v, want %v", config.Headers, headers)
	}
	if !bytes.Equal(config.Body, body) {
		t.Errorf("Body = %q, want %q", config.Body, body)
	}
	if !config.SkipReadResponseBody {
		t.Error("SkipReadResponseBody = false, want true")
	}
	if !config.SkipErrorOnStatus {
		t.Error("SkipErrorOnStatus = false, want true")
	}
	if config.RetryConfig != retryConfig {
		t.Errorf("RetryConfig = %p, want %p", config.RetryConfig, retryConfig)
	}
	if config.HttpClient != httpClient {
		t.Errorf("HttpClient = %p, want %p", config.HttpClient, httpClient)
	}
}

func TestWithHeadersMerges(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		options []Option
		want    map[string]string
	}{
		{
			name: "later options add to earlier headers",
			options: []Option{
				WithHeaders(map[string]string{"Authorization": "Bearer token"}),
				WithHeaders(map[string]string{"Content-Type": "application/json"}),
			},
			want: map[string]string{
				"Authorization": "Bearer token",
				"Content-Type":  "application/json",
			},
		},
		{
			name: "later options overwrite the same name",
			options: []Option{
				WithHeaders(map[string]string{"Accept": "application/xml"}),
				WithHeaders(map[string]string{"Accept": "application/json"}),
			},
			want: map[string]string{"Accept": "application/json"},
		},
		{
			name: "empty headers leave earlier headers intact",
			options: []Option{
				WithHeaders(map[string]string{"Authorization": "Bearer token"}),
				WithHeaders(nil),
			},
			want: map[string]string{"Authorization": "Bearer token"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			config := New(testCase.options...)

			if !maps.Equal(config.Headers, testCase.want) {
				t.Errorf("Headers = %v, want %v", config.Headers, testCase.want)
			}
		})
	}
}

func TestWithHeadersDoesNotAliasInput(t *testing.T) {
	t.Parallel()

	callerHeaders := map[string]string{"Authorization": "Bearer token"}

	config := New(WithHeaders(callerHeaders), WithHeaders(map[string]string{"Accept": "application/json"}))

	if _, ok := callerHeaders["Accept"]; ok {
		t.Error("the caller's header map was mutated")
	}
	if len(config.Headers) != 2 {
		t.Errorf("Headers = %v, want 2 entries", config.Headers)
	}
}
