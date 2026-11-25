package http_context_extractor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	altshiftContext "github.com/altshiftab/utils_go/pkg/context"
	altshiftHttpContext "github.com/altshiftab/utils_go/pkg/http/context"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	"github.com/altshiftab/utils_go/pkg/http/types/http_context_extractor/http_context_extractor_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
	altshiftLog "github.com/altshiftab/utils_go/pkg/log"
	altshiftSchemaTypes "github.com/altshiftab/utils_go/pkg/schema"
)

const relativeTargetToken = "SECRET-TARGET-TOKEN" //nolint:gosec // A made-up value, not a credential.

// withRequest is a context carrying the HTTP context of a request, as the error of a fetch does.
func withRequest(t *testing.T, request *http.Request) context.Context {
	t.Helper()

	return context.WithValue(context.Background(), altshiftHttpContext.HttpContextContextKey, &altshiftHttpTypes.HttpContext{Request: request})
}

func outgoingRequest(t *testing.T, rawUrl string) *http.Request {
	t.Helper()

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawUrl, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	return request
}

// A URL that names no host is for the host of the request the context is for, and the masks scoped
// to that host apply to it; one that names its own host keeps it.
func TestExtractor_MaskUrlStringInContext(t *testing.T) {
	t.Parallel()

	scoped := &Extractor{MaskedUrlParams: []*altshiftSchemaTypes.Url{{Domain: "api.example.com", Query: "token"}}}
	unscoped := &Extractor{MaskedUrlParams: []*altshiftSchemaTypes.Url{{Query: "token"}}}

	incoming := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/start", nil)
	incoming.Host = "api.example.com"

	relative := "/next?token=" + relativeTargetToken
	relativeMasked := "/next?token=%28MASKED%29"

	testCases := []struct {
		name      string
		extractor *Extractor
		// The request the context is for; none for a context without an HTTP context, and
		// withoutContext for no context at all.
		request        *http.Request
		withoutContext bool
		raw            string
		want           string
	}{
		{
			name:      "a relative URL is for the host of an outgoing request",
			extractor: scoped,
			request:   outgoingRequest(t, "https://api.example.com/start"),
			raw:       relative,
			want:      relativeMasked,
		},
		{
			name:      "an incoming request names its host in Host",
			extractor: scoped,
			request:   incoming,
			raw:       relative,
			want:      relativeMasked,
		},
		{
			name:      "a request for another host leaves it",
			extractor: scoped,
			request:   outgoingRequest(t, "https://other.example.org/start"),
			raw:       relative,
			want:      relative,
		},
		{
			name:      "a URL naming another host keeps it",
			extractor: scoped,
			request:   outgoingRequest(t, "https://api.example.com/start"),
			raw:       "https://other.example.org/next?token=" + relativeTargetToken,
			want:      "https://other.example.org/next?token=" + relativeTargetToken,
		},
		{
			name:           "without a context it is MaskUrlString",
			extractor:      scoped,
			withoutContext: true,
			raw:            relative,
			want:           relative,
		},
		{
			name:      "a context without an HTTP context leaves the unscoped masks",
			extractor: unscoped,
			raw:       relative,
			want:      relativeMasked,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			switch {
			case testCase.withoutContext:
				ctx = nil
			case testCase.request != nil:
				ctx = withRequest(t, testCase.request)
			}

			if got := testCase.extractor.MaskUrlStringInContext(ctx, testCase.raw); got != testCase.want {
				t.Errorf("%s: MaskUrlStringInContext(%q) = %q, want %q", testCase.name, testCase.raw, got, testCase.want)
			}
		})
	}
}

var errRedirectRefused = errors.New("redirect refused")

// Go's client writes the Location of a redirect it refuses into its *url.Error as sent, relative or
// not. A mask scoped to the host the request was for masks it in the entry the error is logged in --
// the record's message, the error's own and the URL of its context -- as it masks the same Location
// in the response's headers.
func TestErrorContextExtractor_MasksARefusedRelativeRedirect(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/next?token="+relativeTargetToken)
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(server.Close)

	refusing := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return errRedirectRefused }}

	_, _, err := altshiftHttpUtils.Fetch(t.Context(), server.URL+"/start", fetch_config.WithHttpClient(refusing))
	urlErr, ok := errors.AsType[*url.Error](err)
	if !ok || urlErr.URL != "/next?token="+relativeTargetToken {
		t.Fatalf("the fetch did not fail with the relative target, so the test shows nothing: %v", err)
	}

	// Scoped to the test server's host, which a relative URL can match only by being taken as one
	// for it.
	extractor := New(http_context_extractor_config.WithMaskedUrlParams(&altshiftSchemaTypes.Url{Domain: "127.0.0.1", Query: "token"}))
	errorExtractor := &altshiftLog.ErrorContextExtractor{ContextExtractors: []altshiftLog.ContextExtractor{extractor}}

	record := slog.NewRecord(time.Time{}, slog.LevelError, fmt.Sprintf("the fetch failed: %v", err), 0)
	if err := errorExtractor.Handle(altshiftContext.WithError(context.Background(), err), &record); err != nil {
		t.Fatalf("handle: %v", err)
	}

	var text strings.Builder
	text.WriteString(record.Message)
	record.Attrs(func(attr slog.Attr) bool {
		text.WriteString(" ")
		text.WriteString(attr.String())
		return true
	})

	if strings.Contains(text.String(), relativeTargetToken) {
		t.Errorf("the entry kept the token: %s", text.String())
	}
	if !strings.Contains(record.Message, "%28MASKED%29") {
		t.Errorf("the message was not masked: %s", record.Message)
	}
}
