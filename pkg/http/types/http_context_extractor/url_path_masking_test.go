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
		name    string
		pattern string
		path    string
		want    string
	}{
		{name: "the group", pattern: `:/([^:]+):`, path: "/items/f:/Anna Andersson.pptx:/content", want: "/items/f:/(MASKED):/content"},
		{name: "the whole match, with no group", pattern: `/people/[^/]+`, path: "/api/people/anna/notes", want: "/api(MASKED)/notes"},
		{name: "every match", pattern: `:/([^:]+):`, path: "/a:/one:/b:/two:", want: "/a:/(MASKED):/b:/(MASKED):"},
		{name: "no match", pattern: `:/([^:]+):`, path: "/items/f/content", want: "/items/f/content"},
		{name: "an optional group that did not take part", pattern: `/x(/y)?`, path: "/a/x/b", want: "/a(MASKED)/b"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := maskPathMatches(regexp.MustCompile(testCase.pattern), testCase.path); got != testCase.want {
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
