package upload_config

import (
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		options []Option
		want    Config
	}{
		{name: "defaults", want: Config{BlockSize: DefaultBlockSize}},
		{name: "a nil option", options: []Option{nil}, want: Config{BlockSize: DefaultBlockSize}},
		{
			name: "everything set",
			options: []Option{
				WithIfAbsent(true),
				WithBlockSize(1024),
				WithContentType("application/octet-stream"),
				WithMetadata(map[string]string{"a": "b"}),
			},
			want: Config{IfAbsent: true, BlockSize: 1024, ContentType: "application/octet-stream", Metadata: map[string]string{"a": "b"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			config := New(testCase.options...)
			if config.IfAbsent != testCase.want.IfAbsent || config.BlockSize != testCase.want.BlockSize ||
				config.ContentType != testCase.want.ContentType || len(config.Metadata) != len(testCase.want.Metadata) {
				t.Errorf("config %+v, want %+v", config, testCase.want)
			}
			for name, value := range testCase.want.Metadata {
				if config.Metadata[name] != value {
					t.Errorf("metadata %q is %q, want %q", name, config.Metadata[name], value)
				}
			}
		})
	}
}
