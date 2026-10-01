package graph

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
)

var errUnrelated = errors.New("boom")

func TestPathBuilderUrl(t *testing.T) {
	t.Parallel()

	baseUrl := &url.URL{Scheme: "https", Host: Domain, Path: "/v1.0"}

	testCases := []struct {
		name  string
		build func() *PathBuilder
		query url.Values
		want  string
	}{
		{
			name:  "literal and escaped",
			build: func() *PathBuilder { return new(PathBuilder).Literal("/drives/").Escaped("b!x y").Literal("/root") },
			want:  "https://graph.microsoft.com/v1.0/drives/b%21x%20y/root",
		},
		{
			name: "path addressing keeps colons and escapes segments",
			build: func() *PathBuilder {
				return new(PathBuilder).Literal("/root:/").EscapedPath("/Rapporter//Q3 #1?/år.xlsx/").Literal(":")
			},
			want: "https://graph.microsoft.com/v1.0/root:/Rapporter/Q3%20%231%3F/%C3%A5r.xlsx:",
		},
		{
			name:  "escaped slash stays inside its segment",
			build: func() *PathBuilder { return new(PathBuilder).Literal("/items/").Escaped("a/b") },
			want:  "https://graph.microsoft.com/v1.0/items/a%2Fb",
		},
		{
			name: "odata function syntax kept literal",
			build: func() *PathBuilder {
				return new(PathBuilder).Literal("/range(address='").Escaped(ODataString("A1:B2")).Literal("')")
			},
			want: "https://graph.microsoft.com/v1.0/range(address='A1:B2')",
		},
		{
			name:  "query",
			build: func() *PathBuilder { return new(PathBuilder).Literal("/x") },
			query: url.Values{"@microsoft.graph.conflictBehavior": {"replace"}},
			want:  "https://graph.microsoft.com/v1.0/x?%40microsoft.graph.conflictBehavior=replace",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := testCase.build().Url(baseUrl, testCase.query); got != testCase.want {
				t.Errorf("Url() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestPathSegments(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		path string
		want []string
	}{
		{name: "empty", path: "", want: nil},
		{name: "slashes only", path: "///", want: nil},
		{name: "nested", path: "/a//b/c/", want: []string{"a", "b", "c"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := PathSegments(testCase.path); !slices.Equal(got, testCase.want) {
				t.Errorf("PathSegments(%q) = %q, want %q", testCase.path, got, testCase.want)
			}
		})
	}
}

func TestODataString(t *testing.T) {
	t.Parallel()

	if got := ODataString("'My Sheet'!A1"); got != "''My Sheet''!A1" {
		t.Errorf("ODataString = %q", got)
	}
}

func TestIsStatus(t *testing.T) {
	t.Parallel()

	notFound := altshiftErrors.NewWithTrace(&altshiftHttpErrors.Non2xxStatusCodeError{StatusCode: http.StatusNotFound})

	testCases := []struct {
		name       string
		err        error
		statusCode int
		want       bool
	}{
		{name: "nil", err: nil, statusCode: http.StatusNotFound, want: false},
		{name: "matching wrapped", err: fmt.Errorf("get: %w", notFound), statusCode: http.StatusNotFound, want: true},
		{name: "other status", err: notFound, statusCode: http.StatusConflict, want: false},
		{name: "unrelated", err: errUnrelated, statusCode: http.StatusNotFound, want: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := IsStatus(testCase.err, testCase.statusCode); got != testCase.want {
				t.Errorf("IsStatus() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestNextLinkOrAndExtract(t *testing.T) {
	t.Parallel()

	makeUrl := NextLinkOr("https://first")
	if got := makeUrl(""); got != "https://first" {
		t.Errorf("first page = %q", got)
	}
	if got := makeUrl("https://next"); got != "https://next" {
		t.Errorf("next page = %q", got)
	}

	items, next := Extract(&ListResponse[string]{Value: []string{"a"}, NextLink: "https://next"})
	if !slices.Equal(items, []string{"a"}) || next != "https://next" {
		t.Errorf("Extract = %q, %q", items, next)
	}
}
