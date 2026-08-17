package image_reference

import (
	"errors"
	"reflect"
	"testing"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/missing_error"
	"github.com/altshiftab/utils_go/pkg/schema"
)

func TestParse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected *Reference
	}{
		{
			name:     "registry repository and tag",
			input:    "registry.example.com/library/nginx:1.25",
			expected: &Reference{Registry: "registry.example.com", Repository: "library/nginx", Tag: "1.25"},
		},
		{
			name:     "registry repository no tag",
			input:    "registry.example.com/library/nginx",
			expected: &Reference{Registry: "registry.example.com", Repository: "library/nginx"},
		},
		{
			name:     "digest",
			input:    "registry.example.com/library/nginx@sha256:deadbeef",
			expected: &Reference{Registry: "registry.example.com", Repository: "library/nginx", Digest: "sha256:deadbeef"},
		},
		{
			name:     "registry with port and tag",
			input:    "localhost:5000/myrepo:v1",
			expected: &Reference{Registry: "localhost:5000", Repository: "myrepo", Tag: "v1"},
		},
		{
			name:     "registry with port no tag",
			input:    "localhost:5000/myrepo",
			expected: &Reference{Registry: "localhost:5000", Repository: "myrepo"},
		},
		{
			name:     "tag and digest together",
			input:    "reg.io/app:1.0@sha256:abcd",
			expected: &Reference{Registry: "reg.io", Repository: "app", Tag: "1.0", Digest: "sha256:abcd"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := Parse(testCase.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, testCase.expected) {
				t.Fatalf("expected %+v, got %+v", testCase.expected, got)
			}
		})
	}
}

func TestParseMissingRegistry(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"nginx", "nginx:1.25"} {
		got, err := Parse(input)
		if err == nil {
			t.Fatalf("expected error for %q, got %+v", input, got)
		}
		if _, ok := errors.AsType[*missing_error.Error](err); !ok {
			t.Fatalf("expected missing_error for %q, got %v", input, err)
		}
	}
}

func TestParseEmptyRepository(t *testing.T) {
	t.Parallel()

	got, err := Parse("registry.example.com/")
	if err == nil {
		t.Fatalf("expected error, got %+v", got)
	}
	if _, ok := errors.AsType[*empty_error.Error](err); !ok {
		t.Fatalf("expected empty_error, got %v", err)
	}
}

func TestContainerImage(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		reference *Reference
		expected  *schema.ContainerImage
	}{
		{
			name:      "full reference",
			reference: &Reference{Registry: "reg.io", Repository: "library/nginx", Tag: "1.25", Digest: "sha256:xxx"},
			expected: &schema.ContainerImage{
				Name: "reg.io/library/nginx",
				Tag:  "1.25",
				Hash: &schema.ContainerImageHash{All: []string{"sha256:xxx"}},
			},
		},
		{
			name:      "name only",
			reference: &Reference{Registry: "reg.io", Repository: "app", Tag: "latest"},
			expected:  &schema.ContainerImage{Name: "reg.io/app", Tag: "latest"},
		},
		{
			name:      "digest only",
			reference: &Reference{Digest: "sha256:xxx"},
			expected:  &schema.ContainerImage{Hash: &schema.ContainerImageHash{All: []string{"sha256:xxx"}}},
		},
		{
			name:      "empty reference",
			reference: &Reference{},
			expected:  nil,
		},
		{
			name:      "tag only yields nil",
			reference: &Reference{Tag: "1.0"},
			expected:  nil,
		},
		{
			name:      "registry without repository yields nil",
			reference: &Reference{Registry: "reg.io", Tag: "1.0"},
			expected:  nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := testCase.reference.ContainerImage()
			if !reflect.DeepEqual(got, testCase.expected) {
				t.Fatalf("expected %+v, got %+v", testCase.expected, got)
			}
		})
	}
}

func TestParseContainerImageRoundTrip(t *testing.T) {
	t.Parallel()

	reference, err := Parse("registry.example.com/library/nginx:1.25")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	image := reference.ContainerImage()
	if image == nil {
		t.Fatal("expected non-nil container image")
	}
	if image.Name != "registry.example.com/library/nginx" {
		t.Fatalf("expected name %q, got %q", "registry.example.com/library/nginx", image.Name)
	}
	if image.Tag != "1.25" {
		t.Fatalf("expected tag %q, got %q", "1.25", image.Tag)
	}
	if image.Hash != nil {
		t.Fatalf("expected nil hash, got %+v", image.Hash)
	}
}
