package rest

import (
	"net/url"
	"strings"
)

// Url is base with the segments appended to its path, each escaped once as a single segment, and with the query.
// URL.String escapes Path, so PathEscape output put there would be escaped twice; RawPath carries the escaped form.
func Url(base *url.URL, query url.Values, segments ...string) string {
	escapedSegments := make([]string, len(segments))
	for i, segment := range segments {
		escapedSegments[i] = url.PathEscape(segment)
	}

	path, rawPath := base.Path, base.EscapedPath()
	if len(segments) != 0 && !strings.HasSuffix(path, "/") {
		path += "/"
		rawPath += "/"
	}

	u := *base
	u.Path = path + strings.Join(segments, "/")
	u.RawPath = rawPath + strings.Join(escapedSegments, "/")
	u.RawQuery = query.Encode()
	return u.String()
}
