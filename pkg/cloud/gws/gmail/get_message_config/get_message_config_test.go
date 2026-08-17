package get_message_config

import (
	"net/http"
	"slices"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.Format != "" {
		t.Errorf("default Format = %q, want empty", defaults.Format)
	}
	if len(defaults.MetadataHeaders) != 0 {
		t.Errorf("default MetadataHeaders len = %d, want 0", len(defaults.MetadataHeaders))
	}
	if len(defaults.FetchOptions) != 0 {
		t.Errorf("default FetchOptions len = %d, want 0", len(defaults.FetchOptions))
	}

	config := New(
		WithFormat(FormatFull),
		WithMetadataHeaders("From", "Subject"),
		WithFetchOptions(fetch_config.WithMethod(http.MethodPost)),
	)
	if config.Format != FormatFull {
		t.Errorf("Format = %q, want %q", config.Format, FormatFull)
	}
	if !slices.Equal(config.MetadataHeaders, []string{"From", "Subject"}) {
		t.Errorf("MetadataHeaders = %#v, want [From Subject]", config.MetadataHeaders)
	}
	if len(config.FetchOptions) != 1 {
		t.Fatalf("FetchOptions len = %d, want 1", len(config.FetchOptions))
	}
	if applied := fetch_config.New(config.FetchOptions...); applied.Method != http.MethodPost {
		t.Errorf("applied fetch Method = %q, want %q", applied.Method, http.MethodPost)
	}
}

func TestWithMetadataHeadersAppends(t *testing.T) {
	t.Parallel()

	config := New(
		WithMetadataHeaders("From"),
		WithMetadataHeaders("Subject", "Date"),
	)
	if !slices.Equal(config.MetadataHeaders, []string{"From", "Subject", "Date"}) {
		t.Errorf("MetadataHeaders = %#v, want [From Subject Date]", config.MetadataHeaders)
	}
}

func TestFormatConstants(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		format Format
		want   string
	}{
		{name: "minimal", format: FormatMinimal, want: "minimal"},
		{name: "full", format: FormatFull, want: "full"},
		{name: "raw", format: FormatRaw, want: "raw"},
		{name: "metadata", format: FormatMetadata, want: "metadata"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if string(testCase.format) != testCase.want {
				t.Errorf("Format = %q, want %q", testCase.format, testCase.want)
			}
		})
	}
}
