package list_blobs_config

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
		{name: "no options"},
		{name: "a nil option", options: []Option{nil}},
		{
			name: "everything set",
			options: []Option{
				WithPrefix("prod/"),
				WithIncludeVersions(true),
				WithIncludeMetadata(true),
				WithIncludeImmutabilityPolicy(true),
				WithMarker("m"),
				WithMaxResults(10),
			},
			want: Config{Prefix: "prod/", IncludeVersions: true, IncludeMetadata: true, IncludeImmutabilityPolicy: true, Marker: "m", MaxResults: 10},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if config := New(testCase.options...); *config != testCase.want {
				t.Errorf("config %+v, want %+v", *config, testCase.want)
			}
		})
	}
}
