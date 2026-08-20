package mux

import (
	"testing"

	muxTypesResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
)

func TestPrivateCacheControlHeaders(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    []*muxTypesResponse.HeaderEntry
		expected string
		// The entries belong to the endpoint, so a downgrade must not write
		// into the slice it was handed.
		expectShared bool
	}{
		{
			name: "public is downgraded",
			input: []*muxTypesResponse.HeaderEntry{
				{Name: "Cache-Control", Value: "public, max-age=31356000, immutable"},
			},
			expected: "private, max-age=31356000, immutable",
		},
		{
			name:         "private is left as it is",
			input:        []*muxTypesResponse.HeaderEntry{{Name: "Cache-Control", Value: "private, max-age=60"}},
			expected:     "private, max-age=60",
			expectShared: true,
		},
		{
			name:         "no-store is left as it is",
			input:        []*muxTypesResponse.HeaderEntry{{Name: "Cache-Control", Value: "no-store"}},
			expected:     "no-store",
			expectShared: true,
		},
		{
			name:         "an unparsable header is left as it is",
			input:        []*muxTypesResponse.HeaderEntry{{Name: "Cache-Control", Value: "!!!"}},
			expected:     "!!!",
			expectShared: true,
		},
		{
			name:     "the header name is matched without regard to case",
			input:    []*muxTypesResponse.HeaderEntry{{Name: "cache-control", Value: "public, max-age=1"}},
			expected: "private, max-age=1",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			original := testCase.input[0].Value
			out := privateCacheControlHeaders(testCase.input)

			if out[0].Value != testCase.expected {
				t.Fatalf("got %q, want %q", out[0].Value, testCase.expected)
			}
			if testCase.input[0].Value != original {
				t.Fatalf("the caller's entry was written to: %q became %q", original, testCase.input[0].Value)
			}
			if shared := &out[0] == &testCase.input[0]; shared != testCase.expectShared {
				t.Fatalf("shared backing = %t, want %t", shared, testCase.expectShared)
			}
		})
	}

	t.Run("other headers are carried through untouched", func(t *testing.T) {
		t.Parallel()

		headers := []*muxTypesResponse.HeaderEntry{
			{Name: "Content-Type", Value: "text/css"},
			{Name: "Cache-Control", Value: "public, max-age=1"},
			{Name: "ETag", Value: `"abc"`},
		}

		out := privateCacheControlHeaders(headers)
		if len(out) != 3 || out[0].Value != "text/css" || out[2].Value != `"abc"` {
			t.Fatalf("neighbouring headers changed: %#v", out)
		}
		if out[1].Value != "private, max-age=1" {
			t.Fatalf("got %q", out[1].Value)
		}
	})
}
