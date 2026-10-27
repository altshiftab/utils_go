package http_context_extractor

import (
	"bufio"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	altshiftHttpContext "github.com/altshiftab/utils_go/pkg/http/context"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/http/types/http_context_extractor/http_context_extractor_config"
	altshiftSchemaTypes "github.com/altshiftab/utils_go/pkg/schema"
)

func TestMaskBodyFields(t *testing.T) {
	t.Parallel()

	tokenUrl := &altshiftSchemaTypes.Url{Domain: "token.example.test", Path: "/token"}
	other := &altshiftSchemaTypes.Url{Domain: "other.example.test", Path: "/token"}
	secrets := func(fields ...string) []*http_context_extractor_config.MaskedBodyFields {
		return []*http_context_extractor_config.MaskedBodyFields{{Url: tokenUrl, Fields: fields}}
	}

	const (
		json = "application/json"
		form = "application/x-www-form-urlencoded"
	)

	testCases := []struct {
		name        string
		entries     []*http_context_extractor_config.MaskedBodyFields
		url         *altshiftSchemaTypes.Url
		contentType string
		content     string
		want        string
	}{
		{
			name:        "a JSON member, the others kept in their order",
			entries:     secrets("access_token", "id_token"),
			contentType: json,
			content:     `{"token_type":"Bearer","access_token":"at","expires_in":3599,"id_token":"it"}`,
			want:        `{"token_type":"Bearer","access_token":"(MASKED)","expires_in":3599,"id_token":"(MASKED)"}`,
		},
		{
			name:        "a JSON error, which names nothing masked",
			entries:     secrets("access_token"),
			contentType: json,
			content:     `{"error":"invalid_grant","error_description":"Bad Request"}`,
			want:        `{"error":"invalid_grant","error_description":"Bad Request"}`,
		},
		{
			name:        "a JSON member at depth, and one whose value is not a string",
			entries:     secrets("raw", "payload"),
			contentType: json,
			content:     `{"messages":[{"id":"1","raw":"cmF3"}],"payload":{"headers":[{"name":"To"}]}}`,
			want:        `{"messages":[{"id":"1","raw":"(MASKED)"}],"payload":"(MASKED)"}`,
		},
		{
			name:        "a JSON member of the name as a value is no member",
			entries:     secrets("code"),
			contentType: json,
			content:     `{"name":"code","code":"c"}`,
			want:        `{"name":"code","code":"(MASKED)"}`,
		},
		{
			name:        "JSON with a charset",
			entries:     secrets("code"),
			contentType: "application/json; charset=utf-8",
			content:     `{"code":"c"}`,
			want:        `{"code":"(MASKED)"}`,
		},
		{
			name:        "a JSON media type by its suffix",
			entries:     secrets("code"),
			contentType: "application/problem+json",
			content:     `{"code":"c"}`,
			want:        `{"code":"(MASKED)"}`,
		},
		{
			name:        "a form key, the others kept in their order",
			entries:     secrets("client_secret", "code"),
			contentType: form,
			content:     "grant_type=authorization_code&client_secret=s3cret&code=c0de&redirect_uri=https%3A%2F%2Fx",
			want:        "grant_type=authorization_code&client_secret=%28MASKED%29&code=%28MASKED%29&redirect_uri=https%3A%2F%2Fx",
		},
		{
			name:        "a form key written escaped",
			entries:     secrets("client_secret"),
			contentType: form,
			content:     "client%5Fsecret=s3cret",
			want:        "client%5Fsecret=%28MASKED%29",
		},
		{
			// url.ParseQuery reads JSON as a form with one odd key; it is no form.
			name:        "JSON labelled a form",
			entries:     secrets("access_token"),
			contentType: form,
			content:     `{"access_token":"at"}`,
			want:        maskedValue,
		},
		{
			// An = in a value gives the pair its separator; the key is then the JSON before it.
			name:        "JSON with an = in it labelled a form",
			entries:     secrets("access_token"),
			contentType: form,
			content:     `{"access_token":"at=1"}`,
			want:        maskedValue,
		},
		{name: "a word labelled a form", entries: secrets("code"), contentType: form, content: "justaword", want: maskedValue},
		{name: "a form with an empty pair", entries: secrets("code"), contentType: form, content: "code=c&", want: maskedValue},
		{name: "a form key of other characters", entries: secrets("code"), contentType: form, content: "code=c&a%40b=1", want: maskedValue},
		{name: "JSON that does not parse", entries: secrets("code"), contentType: json, content: `{"code":"c"`, want: maskedValue},
		{
			// A body cut at the limit is no longer whole JSON, and what was cut off may well name
			// what is masked.
			name:        "JSON cut at the limit",
			entries:     secrets("code"),
			contentType: json,
			content:     `{"a":"b","code":"c... [truncated, 9000 bytes in full]`,
			want:        maskedValue,
		},
		{name: "JSON naming a member twice", entries: secrets("x"), contentType: json, content: `{"code":"a","code":"b"}`, want: maskedValue},
		{name: "a form that does not parse", entries: secrets("code"), contentType: form, content: "code=%zz", want: maskedValue},
		{name: "a body of another type", entries: secrets("code"), contentType: "text/plain", content: "code=c", want: maskedValue},
		{name: "a body of no stated type", entries: secrets("code"), contentType: "", content: `{"code":"c"}`, want: maskedValue},
		{name: "an entry naming no field", entries: secrets(), contentType: json, content: `{"a":"b"}`, want: maskedValue},
		{
			name:        "a URL no entry matches",
			entries:     secrets("code"),
			url:         other,
			contentType: json,
			content:     `{"code":"c"}`,
			want:        `{"code":"c"}`,
		},
		{
			name: "two entries matching",
			entries: append(
				secrets("code"),
				&http_context_extractor_config.MaskedBodyFields{Url: tokenUrl, Fields: []string{"client_secret"}},
			),
			contentType: form,
			content:     "code=c&client_secret=s&state=x",
			want:        "code=%28MASKED%29&client_secret=%28MASKED%29&state=x",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			u := testCase.url
			if u == nil {
				u = tokenUrl
			}
			if got := maskBodyFields(testCase.entries, u, testCase.content, testCase.contentType); got != testCase.want {
				t.Errorf("%s: got %s, want %s", testCase.name, got, testCase.want)
			}
		})
	}
}

/*
 * A failed token exchange, written out as the log writes the context a failed fetch carries: the
 * credentials in both bodies are masked, and what says why it failed is kept. A whole-body mask on
 * the same URL still masks the body whole.
 */
func TestExtractor_Handle_MasksBodyFields(t *testing.T) {
	t.Parallel()

	const (
		clientSecret = "a-test-client-secret" //nolint:gosec // A made-up secret.
		code         = "a-test-code"
		accessToken  = "a-test-access-token"
	)
	tokenUrl := &altshiftSchemaTypes.Url{Domain: "token.example.test", Path: "/token"}
	fieldMasks := func() *Extractor {
		return New(
			http_context_extractor_config.WithMaskedRequestBodyFields(
				&http_context_extractor_config.MaskedBodyFields{Url: tokenUrl, Fields: []string{"client_secret", "code"}},
			),
			http_context_extractor_config.WithMaskedResponseBodyFields(
				&http_context_extractor_config.MaskedBodyFields{Url: tokenUrl, Fields: []string{"access_token"}},
			),
		)
	}
	bothMasks := fieldMasks()
	bothMasks.MaskedResponseBodyUrls = []*altshiftSchemaTypes.Url{tokenUrl}

	testCases := []struct {
		name      string
		extractor *Extractor
		wantGone  []string
		wantKept  []string
	}{
		{
			name:      "field masks",
			extractor: fieldMasks(),
			wantGone:  []string{clientSecret, code, accessToken},
			wantKept:  []string{"grant_type=authorization_code", "invalid_scope", "token_type"},
		},
		{
			name:      "a whole-body mask beside them",
			extractor: bothMasks,
			wantGone:  []string{clientSecret, code, accessToken, "invalid_scope"},
			wantKept:  []string{"grant_type=authorization_code"},
		},
		{
			name:      "no masks",
			extractor: New(),
			wantKept:  []string{clientSecret, code, accessToken},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			request, err := http.ReadRequest(bufio.NewReader(strings.NewReader(
				"POST /token HTTP/1.1\r\nHost: token.example.test\r\nContent-Type: application/x-www-form-urlencoded\r\n\r\n",
			)))
			if err != nil {
				t.Fatalf("%s: read request: %v", testCase.name, err)
			}

			record := &slog.Record{Message: "An HTTP request failed."}
			ctx := altshiftHttpContext.WithHttpContextValue(context.Background(), &altshiftHttpTypes.HttpContext{
				Request:     request,
				RequestBody: []byte("grant_type=authorization_code&client_secret=" + clientSecret + "&code=" + code),
				Response: &http.Response{
					StatusCode: http.StatusBadRequest,
					Header:     http.Header{"Content-Type": {"application/json; charset=utf-8"}},
				},
				ResponseBody: []byte(`{"error":"invalid_scope","access_token":"` + accessToken + `","token_type":"Bearer"}`),
			})
			if err := testCase.extractor.Handle(ctx, record); err != nil {
				t.Fatalf("%s: handle: %v", testCase.name, err)
			}

			text := recordText(t, record)
			for _, gone := range testCase.wantGone {
				if strings.Contains(text, gone) {
					t.Errorf("%s: %q is in the entry: %s", testCase.name, gone, text)
				}
			}
			for _, kept := range testCase.wantKept {
				if !strings.Contains(text, kept) {
					t.Errorf("%s: %q is not in the entry: %s", testCase.name, kept, text)
				}
			}
		})
	}
}
