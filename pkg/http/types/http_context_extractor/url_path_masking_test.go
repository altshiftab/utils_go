package http_context_extractor

import (
	"bufio"
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"testing"

	altshiftHttpContext "github.com/altshiftab/utils_go/pkg/http/context"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/http/types/http_context_extractor/http_context_extractor_config"
	altshiftSchemaTypes "github.com/altshiftab/utils_go/pkg/schema"
)

// The name a file was uploaded under, which Microsoft Graph addresses it by in the URL.
const (
	pathTestName   = "Anna Andersson 2026-10-06.pptx"
	pathTestDomain = "graph.microsoft.com"
)

var graphItemPath = regexp.MustCompile(`:/([^:]+):`)

// graphPathExtractor masks the item path of every URL on Graph.
func graphPathExtractor() *Extractor {
	return New(http_context_extractor_config.WithMaskedUrlPaths(&http_context_extractor_config.MaskedUrlPath{
		Url:     &altshiftSchemaTypes.Url{Domain: pathTestDomain},
		Pattern: graphItemPath,
	}))
}

func TestMaskPathMatches(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		patterns []string
		path     string
		want     string
	}{
		{name: "the group", patterns: []string{`:/([^:]+):`}, path: "/items/f:/Anna%20Andersson.pptx:/content", want: "/items/f:/(MASKED):/content"},
		{name: "the whole match, with no group", patterns: []string{`/people/[^/]+`}, path: "/api/people/anna/notes", want: "/api(MASKED)/notes"},
		{name: "every match", patterns: []string{`:/([^:]+):`}, path: "/a:/one:/b:/two:", want: "/a:/(MASKED):/b:/(MASKED):"},
		{name: "no match", patterns: []string{`:/([^:]+):`}, path: "/items/f/content", want: "/items/f/content"},
		{name: "an optional group that did not take part", patterns: []string{`/x(/y)?`}, path: "/a/x/b", want: "/a(MASKED)/b"},
		{
			// Each pattern sees the path as it was, so the one masking a prefix does not hide the
			// file name from the other, whichever is declared first.
			name:     "overlapping patterns, in one order",
			patterns: []string{`/people/[^/]+`, `/people/[^/]+/files/([^/]+)`},
			path:     "/api/people/anna/files/report-anna.pdf",
			want:     "/api(MASKED)/files/(MASKED)",
		},
		{
			name:     "overlapping patterns, in the other",
			patterns: []string{`/people/[^/]+/files/([^/]+)`, `/people/[^/]+`},
			path:     "/api/people/anna/files/report-anna.pdf",
			want:     "/api(MASKED)/files/(MASKED)",
		},
		{
			// Matched as written, an encoded slash is inside the segment it was written in.
			name:     "an encoded delimiter inside the name",
			patterns: []string{`/people/([^/]+)`},
			path:     "/api/people/Anna%2FAndersson/notes",
			want:     "/api/people/(MASKED)/notes",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			patterns := make([]*regexp.Regexp, 0, len(testCase.patterns))
			for _, pattern := range testCase.patterns {
				patterns = append(patterns, regexp.MustCompile(pattern))
			}
			if got := maskPathMatches(patterns, testCase.path); got != testCase.want {
				t.Errorf("got %q, want %q", got, testCase.want)
			}
		})
	}
}

// The path is masked in every field of the URL that carries it, decoded and encoded alike; a URL on
// another host is left as it is; and a query mask declared on the same path still applies, its
// pattern matched on the path the URL arrived with.
func TestExtractor_MaskUrl_Path(t *testing.T) {
	t.Parallel()

	encoded := "https://graph.microsoft.com/v1.0/drives/d/items/f:/Anna%20Andersson%202026-10-06.pptx:/content?token=SECRET-TOKEN"
	decodedPath := "/v1.0/drives/d/items/f:/" + pathTestName + ":/content"

	extractor := graphPathExtractor()
	extractor.MaskedUrlParams = []*altshiftSchemaTypes.Url{{Domain: pathTestDomain, Path: decodedPath, Query: "token"}}

	testCases := []struct {
		name     string
		url      *altshiftSchemaTypes.Url
		wantGone []string
		wantKept []string
	}{
		{
			name:     "on Graph",
			url:      &altshiftSchemaTypes.Url{Domain: pathTestDomain, Path: decodedPath, Full: encoded, Original: encoded, Query: "token=SECRET-TOKEN"},
			wantGone: []string{"Anna", "SECRET-TOKEN"},
			wantKept: []string{"/v1.0/drives/d/items/f:/", ":/content"},
		},
		{
			name:     "elsewhere",
			url:      &altshiftSchemaTypes.Url{Domain: "example.com", Path: decodedPath, Full: "https://example.com/items/f:/Anna%20Andersson:/content"},
			wantKept: []string{"Anna"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			extractor.maskUrl(testCase.url)

			fields := strings.Join([]string{testCase.url.Path, testCase.url.Full, testCase.url.Original, testCase.url.Query}, "\n")
			for _, gone := range testCase.wantGone {
				if strings.Contains(fields, gone) {
					t.Errorf("%q is still in the URL:\n%s", gone, fields)
				}
			}
			for _, kept := range testCase.wantKept {
				if !strings.Contains(fields, kept) {
					t.Errorf("%q was lost from the URL:\n%s", kept, fields)
				}
			}
		})
	}
}

// A request that never reached its server fails with an error whose message is the URL it was for,
// which the record's message carries -- and the request's own URL is in the entry as its fields. The
// name is in neither.
func TestExtractor_Handle_MasksUrlPaths(t *testing.T) {
	t.Parallel()

	request, err := http.ReadRequest(bufio.NewReader(strings.NewReader(
		"PUT /v1.0/drives/d/items/f:/Anna%20Andersson%202026-10-06.pptx:/content HTTP/1.1\r\nHost: graph.microsoft.com\r\n" +
			"Referer: https://graph.microsoft.com/v1.0/drives/d/items/f:/Anna%20Andersson%202026-10-06.pptx:\r\n\r\n",
	)))
	if err != nil {
		t.Fatalf("read request: %v", err)
	}

	record := &slog.Record{
		Message: `Put "https://graph.microsoft.com/v1.0/drives/d/items/f:/Anna%20Andersson%202026-10-06.pptx:/content": dial tcp: connection refused`,
	}
	ctx := altshiftHttpContext.WithHttpContextValue(
		context.Background(),
		&altshiftHttpTypes.HttpContext{
			Request: request,
			Response: &http.Response{
				StatusCode: http.StatusCreated,
				Header:     http.Header{"Location": {"https://graph.microsoft.com/v1.0/drives/d/items/f:/Anna%20Andersson%202026-10-06.pptx:"}},
			},
		},
	)

	if err := graphPathExtractor().Handle(ctx, record); err != nil {
		t.Fatalf("handle: %v", err)
	}

	text := record.Message + "\n" + recordText(t, record)
	if strings.Contains(text, "Anna") {
		t.Errorf("the name is in the entry: %s", text)
	}
	if !strings.Contains(text, "/v1.0/drives/d/items/f:/") {
		t.Errorf("the entry lost what was asked for: %s", text)
	}
}

// Where a path is masked, a URL with no query is masked all the same: the shortcut that leaves such a
// URL alone holds only where every mask is a query's.
func TestExtractor_MaskUrlString_Path(t *testing.T) {
	t.Parallel()

	raw := "https://graph.microsoft.com/v1.0/drives/d/items/f:/Anna%20Andersson.pptx:/content"
	if got := graphPathExtractor().MaskUrlString(raw); strings.Contains(got, "Anna") {
		t.Errorf("got %q, want the name masked", got)
	}
	if got := graphPathExtractor().MaskText("Put " + raw + " failed"); strings.Contains(got, "Anna") {
		t.Errorf("got %q, want the name masked", got)
	}

	// An extractor masking queries alone leaves a URL without one as it was written, unread.
	queryOnly := &Extractor{MaskedUrlParams: []*altshiftSchemaTypes.Url{{Query: "token"}}}
	if got := queryOnly.MaskUrlString(raw); got != raw {
		t.Errorf("got %q, want %q untouched", got, raw)
	}
}

// An extractor that is nil, held as a text masker, masks nothing and does not fail.
func TestExtractor_MaskText_Nil(t *testing.T) {
	t.Parallel()

	var extractor *Extractor
	if got := extractor.MaskText("a text"); got != "a text" {
		t.Errorf("got %q", got)
	}
}

// A body mask is matched on the URL as it arrived, as a header mask is: a path mask replacing the
// very prefix it names does not turn it off.
func TestExtractor_Handle_PathMaskLeavesBodyMasks(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		extractor *Extractor
		request   string
	}{
		{
			name: "a pattern with no group",
			extractor: New(
				http_context_extractor_config.WithMaskedUrlPaths(&http_context_extractor_config.MaskedUrlPath{Pattern: regexp.MustCompile(`/people/[^/]+`)}),
				http_context_extractor_config.WithMaskedRequestBodyUrls(&altshiftSchemaTypes.Url{Path: "/api/people/*"}),
			),
			request: "POST /api/people/anna/notes HTTP/1.1\r\nHost: h.example\r\nContent-Type: application/json\r\n\r\n",
		},
		{
			name: "a field mask",
			extractor: New(
				http_context_extractor_config.WithMaskedUrlPaths(&http_context_extractor_config.MaskedUrlPath{Pattern: regexp.MustCompile(`/people/[^/]+`)}),
				http_context_extractor_config.WithMaskedRequestBodyFields(&http_context_extractor_config.MaskedBodyFields{
					Url: &altshiftSchemaTypes.Url{Path: "/api/people/*"}, Fields: []string{"note"},
				}),
			),
			request: "POST /api/people/anna/notes HTTP/1.1\r\nHost: h.example\r\nContent-Type: application/json\r\n\r\n",
		},
		{
			name: "the Graph pattern",
			extractor: New(
				http_context_extractor_config.WithMaskedUrlPaths(&http_context_extractor_config.MaskedUrlPath{
					Url: &altshiftSchemaTypes.Url{Domain: pathTestDomain}, Pattern: graphItemPath,
				}),
				http_context_extractor_config.WithMaskedRequestBodyUrls(&altshiftSchemaTypes.Url{Domain: pathTestDomain, Path: "/v1.0/drives/d/root:/Uploads/*"}),
			),
			request: "PUT /v1.0/drives/d/root:/Uploads/Anna.pdf:/content HTTP/1.1\r\nHost: graph.microsoft.com\r\nContent-Type: application/json\r\n\r\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			request, err := http.ReadRequest(bufio.NewReader(strings.NewReader(testCase.request)))
			if err != nil {
				t.Fatalf("read request: %v", err)
			}

			record := &slog.Record{Message: "A fetch was performed."}
			ctx := altshiftHttpContext.WithHttpContextValue(
				context.Background(),
				&altshiftHttpTypes.HttpContext{Request: request, RequestBody: []byte(`{"note":"SECRET-BODY"}`)},
			)
			if err := testCase.extractor.Handle(ctx, record); err != nil {
				t.Fatalf("handle: %v", err)
			}

			text := recordText(t, record)
			if !strings.Contains(text, "(MASKED)") {
				t.Fatalf("nothing was masked, so the test shows nothing: %s", text)
			}
			if strings.Contains(text, "SECRET-BODY") {
				t.Errorf("the body is in the entry: %s", text)
			}
		})
	}
}

// Where a path is masked, a URL that cannot be read is replaced whole: which host it is for and what
// its path says cannot be known. Where only queries are masked it is left as it was. A URL no pattern
// matches, on a host one is declared for, comes back as it was written, byte for byte.
func TestExtractor_MaskUrlString_Unreadable(t *testing.T) {
	t.Parallel()

	unreadable := "https://graph.microsoft.com/v1.0/drives/d/root:/Anna Andersson 100%.pptx:/content"
	if got := graphPathExtractor().MaskUrlString(unreadable); got != maskedValue {
		t.Errorf("got %q, want it replaced whole", got)
	}

	queryOnly := &Extractor{MaskedUrlParams: []*altshiftSchemaTypes.Url{{Query: "token"}}}
	if got := queryOnly.MaskUrlString(unreadable + "?token=x"); got != unreadable+"?token=x" {
		t.Errorf("got %q, want it as it was", got)
	}

	untouched := "https://graph.microsoft.com/v1.0/drives/d/items/a%2Fb/x!y"
	if got := graphPathExtractor().MaskUrlString(untouched); got != untouched {
		t.Errorf("got %q, want %q byte for byte", got, untouched)
	}

	var nilExtractor *Extractor
	if got := nilExtractor.MaskUrlString(unreadable); got != unreadable {
		t.Errorf("a nil extractor masked %q", got)
	}
}

// A name holding an encoded delimiter is masked whole in every field: the pattern reads the path as
// it was written, where %2F is inside the segment rather than the end of it.
func TestExtractor_MaskUrl_EncodedDelimiter(t *testing.T) {
	t.Parallel()

	extractor := New(http_context_extractor_config.WithMaskedUrlPaths(&http_context_extractor_config.MaskedUrlPath{
		Pattern: regexp.MustCompile(`/people/([^/]+)`),
	}))
	written := "https://h.example/api/people/Anna%2FAndersson/notes"
	urlStruct := &altshiftSchemaTypes.Url{Domain: "h.example", Path: "/api/people/Anna/Andersson/notes", Full: written, Original: written}

	extractor.maskUrl(urlStruct)

	fields := strings.Join([]string{urlStruct.Path, urlStruct.Full, urlStruct.Original}, "\n")
	if strings.Contains(fields, "Andersson") {
		t.Errorf("part of the name is in the URL:\n%s", fields)
	}
	if !strings.Contains(urlStruct.Full, "/api/people/") || !strings.Contains(urlStruct.Full, "/notes") {
		t.Errorf("the URL lost what was around the name: %s", urlStruct.Full)
	}
}
