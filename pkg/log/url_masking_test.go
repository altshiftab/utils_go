package log

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"testing"
	"time"

	context2 "github.com/altshiftab/utils_go/pkg/context"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// requestMaskingExtractor masks a word in a URL only when asked with the context of the request it
// was made for -- the one ctxKey names -- as a mask scoped to that request's host would, and not
// when asked without a context.
type requestMaskingExtractor struct {
	word    string
	request string
}

func (requestMaskingExtractor) Handle(context.Context, *slog.Record) error { return nil }

func (requestMaskingExtractor) MaskUrlString(raw string) string { return raw }

func (extractor requestMaskingExtractor) MaskUrlStringInContext(ctx context.Context, raw string) string {
	if request, ok := ctx.Value(ctxKey).(string); !ok || request != extractor.request {
		return raw
	}

	return strings.ReplaceAll(raw, extractor.word, "(MASKED)")
}

// The URL a *url.Error carries is masked relative to the request of the context the error carries,
// its own request's, wherever it is written: the record's message, the error's message and the URL
// of its context. The logging context is not asked, its request being perhaps another one.
func TestErrorContextExtractorMasksUrlsInTheErrorsContext(t *testing.T) {
	t.Parallel()

	const (
		word    = "SECRET-WORD"
		request = "the request"
	)
	extractor := &ErrorContextExtractor{ContextExtractors: []ContextExtractor{requestMaskingExtractor{word: word, request: request}}}

	relative := &url.Error{Op: "Get", URL: "/next?token=" + word, Err: errMaskingTest}
	requestCtx := context.WithValue(context.Background(), ctxKey, request)

	testCases := []struct {
		name string
		err  error
		// What the logging context carries under ctxKey; nothing where empty.
		loggingRequest string
		masked         bool
	}{
		{
			name:           "the error carries its request",
			err:            altshiftErrors.NewCtx(requestCtx, relative),
			loggingRequest: "another request",
			masked:         true,
		},
		{
			// The *url.Error is a cause further down, carrying no context of its own: its message
			// is masked relative to the request of the error wrapping it.
			name:   "the error carries its request, the *url.Error a cause of it",
			err:    altshiftErrors.NewCtx(requestCtx, altshiftErrors.New(fmt.Errorf("fetch: %w", relative))),
			masked: true,
		},
		{
			name:           "only the logging context carries it",
			err:            relative,
			loggingRequest: request,
			masked:         false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			record := slog.NewRecord(time.Time{}, slog.LevelError, fmt.Sprintf("failed: %v", testCase.err), 0)
			loggingCtx := context.Background()
			if testCase.loggingRequest != "" {
				loggingCtx = context.WithValue(loggingCtx, ctxKey, testCase.loggingRequest)
			}

			if err := extractor.Handle(context2.WithError(loggingCtx, testCase.err), &record); err != nil {
				t.Fatalf("handle: %v", err)
			}

			var attrs strings.Builder
			record.Attrs(func(attr slog.Attr) bool {
				attrs.WriteString(attr.String())
				return true
			})

			for _, where := range []struct{ name, text string }{
				{name: "the message", text: record.Message},
				{name: "the attributes", text: attrs.String()},
			} {
				if kept := strings.Contains(where.text, word); kept == testCase.masked {
					t.Errorf("%s: %s kept the word = %v, want %v: %s", testCase.name, where.name, kept, !testCase.masked, where.text)
				}
			}
			if testCase.masked && strings.Count(attrs.String(), "(MASKED)") < 2 {
				t.Errorf("%s: want the error's message and the URL of its context masked: %s", testCase.name, attrs.String())
			}
		})
	}
}
