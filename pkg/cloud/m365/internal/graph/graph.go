// Package graph holds what the Microsoft Graph client packages share: the base
// URL, escaped path construction, and @odata.nextLink pagination.
package graph

import (
	"errors"
	"net/url"
	"strings"

	altshiftHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
)

const Domain = "graph.microsoft.com"

var DefaultBaseUrl = &url.URL{Scheme: "https", Host: Domain}

// ListResponse is the envelope of a Graph collection.
type ListResponse[T any] struct {
	Value    []T    `json:"value,omitzero"`
	NextLink string `json:"@odata.nextLink,omitzero"`
}

// Extract adapts ListResponse to rest.ListPaginated, the next link standing in
// for the page token.
func Extract[T any](response *ListResponse[T]) ([]T, string) {
	return response.Value, response.NextLink
}

// NextLinkOr makes a rest.ListPaginated URL function: the first page is
// firstUrl, later pages the absolute @odata.nextLink of the previous one.
func NextLinkOr(firstUrl string) func(nextLink string) string {
	return func(nextLink string) string {
		if nextLink != "" {
			return nextLink
		}
		return firstUrl
	}
}

// PathBuilder builds a URL path whose literal parts (Graph's ":" path
// addressing, OData function syntax) are kept verbatim while the parts taken
// from callers are escaped, keeping Path and RawPath in step.
type PathBuilder struct {
	path    strings.Builder
	rawPath strings.Builder
}

func (b *PathBuilder) Literal(s string) *PathBuilder {
	b.path.WriteString(s)
	b.rawPath.WriteString(s)
	return b
}

func (b *PathBuilder) Escaped(s string) *PathBuilder {
	b.path.WriteString(s)
	b.rawPath.WriteString(url.PathEscape(s))
	return b
}

// EscapedPath escapes each "/"-separated segment of a drive-relative path,
// dropping empty segments so that leading, trailing and doubled slashes are
// harmless.
func (b *PathBuilder) EscapedPath(path string) *PathBuilder {
	for i, segment := range PathSegments(path) {
		if i != 0 {
			b.Literal("/")
		}
		b.Escaped(segment)
	}
	return b
}

// PathSegments splits a "/"-separated path into its non-empty segments.
func PathSegments(path string) []string {
	var segments []string
	for segment := range strings.SplitSeq(path, "/") {
		if segment != "" {
			segments = append(segments, segment)
		}
	}
	return segments
}

// Url resolves the built path against baseUrl.
func (b *PathBuilder) Url(baseUrl *url.URL, query url.Values) string {
	u := *baseUrl
	u.Path = baseUrl.Path + b.path.String()
	u.RawPath = baseUrl.EscapedPath() + b.rawPath.String()
	if len(query) != 0 {
		u.RawQuery = query.Encode()
	}
	return u.String()
}

// ODataString escapes s for use inside a single-quoted OData string literal.
func ODataString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// IsStatus reports whether err stems from a response with the status code.
func IsStatus(err error, statusCode int) bool {
	statusErr, ok := errors.AsType[*altshiftHttpErrors.Non2xxStatusCodeError](err)
	return ok && statusErr.StatusCode == statusCode
}
