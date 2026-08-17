package static_content

import (
	muxTypesResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
)

type StaticContentData struct {
	Data         []byte
	Etag         string
	LastModified string
	Headers      []*muxTypesResponse.HeaderEntry
}

type StaticContent struct {
	StaticContentData
	ContentEncodingToData map[string]*StaticContentData
	// InlineScriptHashes holds Content Security Policy hash sources (e.g.
	// "sha256-<base64>") of inline scripts occurring in HTML content, to be
	// merged into the effective script-src directive when responding.
	InlineScriptHashes []string
}
