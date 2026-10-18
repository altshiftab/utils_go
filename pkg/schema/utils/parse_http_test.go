package utils

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	altshiftJson "github.com/altshiftab/utils_go/pkg/json"
	"github.com/altshiftab/utils_go/pkg/schema"
)

func TestParseHttpBodies(t *testing.T) {
	t.Parallel()

	longText := strings.Repeat("y", 3*DefaultBodyLimit)
	// "å" is two bytes; the limit lands between them.
	cutCharacter := strings.Repeat("a", DefaultBodyLimit-1) + "åtkomst nekad"
	gzipData := []byte{0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03}

	testCases := []struct {
		name        string
		data        []byte
		bodyLimit   int
		wantNil     bool
		wantContent string
	}{
		{name: "no body", data: nil, wantNil: true},
		{name: "text within the limit", data: []byte("<html>blocked</html>"), wantContent: "<html>blocked</html>"},
		{
			name:        "text past the limit is cut and marked",
			data:        []byte(longText),
			wantContent: longText[:DefaultBodyLimit] + fmt.Sprintf("... [truncated, %d bytes in full]", len(longText)),
		},
		{
			name:        "a cut is not made inside a character",
			data:        []byte(cutCharacter),
			wantContent: cutCharacter[:DefaultBodyLimit-1] + fmt.Sprintf("... [truncated, %d bytes in full]", len(cutCharacter)),
		},
		{
			name:        "text that is not UTF-8 is made so",
			data:        []byte("\xe5tkomst nekad"),
			wantContent: "�tkomst nekad",
		},
		{
			name:        "a binary body is rendered by its size",
			data:        gzipData,
			wantContent: fmt.Sprintf("[binary, %d bytes]", len(gzipData)),
		},
		{
			name:        "text that begins as a bitmap does",
			data:        []byte("BMW is a car"),
			wantContent: "BMW is a car",
		},
		{
			name:        "text that begins as an MP3 tag does",
			data:        []byte("ID3 tags name the track"),
			wantContent: "ID3 tags name the track",
		},
		{
			name:        "a limit of its own",
			data:        []byte("0123456789"),
			bodyLimit:   4,
			wantContent: "0123... [truncated, 10 bytes in full]",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://example.com/upload", nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			response := &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}}
			original := bytes.Clone(testCase.data)

			base, err := ParseHttpContextWithBodyLimit(
				&altshiftHttpTypes.HttpContext{
					Request:      request,
					RequestBody:  testCase.data,
					Response:     response,
					ResponseBody: testCase.data,
				},
				testCase.bodyLimit,
			)
			if err != nil {
				t.Fatalf("parse http context: %v", err)
			}
			if base == nil || base.Http == nil || base.Http.Request == nil || base.Http.Response == nil {
				t.Fatalf("expected a request and a response, got %+v", base)
			}

			requestBody, responseBody := base.Http.Request.Body, base.Http.Response.Body
			if testCase.wantNil {
				if requestBody != nil || responseBody != nil {
					t.Errorf("expected no bodies, got %+v and %+v", requestBody, responseBody)
				}
				return
			}

			for _, rendered := range []struct {
				name string
				body *schema.Body
			}{
				{name: "request", body: requestBody},
				{name: "response", body: responseBody},
			} {
				if rendered.body == nil {
					t.Fatalf("%s: expected a body", rendered.name)
				}
				if rendered.body.Bytes != len(testCase.data) {
					t.Errorf("%s: expected %d bytes, got %d", rendered.name, len(testCase.data), rendered.body.Bytes)
				}
				if rendered.body.Content != testCase.wantContent {
					t.Errorf("%s: expected content %q, got %q", rendered.name, testCase.wantContent, rendered.body.Content)
				}
			}

			// What made a whole log entry disappear: the entry must marshal.
			if _, err := altshiftJson.ObjectToMap(base); err != nil {
				t.Errorf("object to map: %v", err)
			}
			if !bytes.Equal(testCase.data, original) {
				t.Error("expected the body to be left as it is")
			}
		})
	}
}

func TestParseHttpMasksUrlPassword(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		rawUrl       string
		wantOriginal string
		wantUsername string
		wantPassword string
	}{
		{ //nolint:gosec // G101: a fixture whose password is what gets masked.
			name:         "a password is masked",
			rawUrl:       "https://user:hunter2@feeds.example.com/rss?key=value",
			wantOriginal: "https://user:(MASKED)@feeds.example.com/rss?key=value",
			wantUsername: "user",
			wantPassword: "(MASKED)",
		},
		{ //nolint:gosec // G101: a fixture whose password is what gets masked.
			name:         "an empty password is masked as present",
			rawUrl:       "https://user:@feeds.example.com/rss",
			wantOriginal: "https://user:(MASKED)@feeds.example.com/rss",
			wantUsername: "user",
			wantPassword: "(MASKED)",
		},
		{
			name:         "a username alone is kept",
			rawUrl:       "https://user@feeds.example.com/rss",
			wantOriginal: "https://user@feeds.example.com/rss",
			wantUsername: "user",
		},
		{
			name:         "an address without user information is kept",
			rawUrl:       "https://feeds.example.com/rss",
			wantOriginal: "https://feeds.example.com/rss",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			requestUrl, err := url.Parse(testCase.rawUrl)
			if err != nil {
				t.Fatalf("url parse: %v", err)
			}

			base, err := ParseHttp(&http.Request{Method: http.MethodGet, URL: requestUrl, Header: http.Header{}}, nil, nil, nil)
			if err != nil {
				t.Fatalf("parse http: %v", err)
			}
			if base == nil || base.Url == nil {
				t.Fatalf("expected a url, got %+v", base)
			}

			if base.Url.Original != testCase.wantOriginal {
				t.Errorf("expected original %q, got %q", testCase.wantOriginal, base.Url.Original)
			}
			if base.Url.Username != testCase.wantUsername {
				t.Errorf("expected username %q, got %q", testCase.wantUsername, base.Url.Username)
			}
			if base.Url.Password != testCase.wantPassword {
				t.Errorf("expected password %q, got %q", testCase.wantPassword, base.Url.Password)
			}
			if message := MakeHttpMessage(base); strings.Contains(message, "hunter2") {
				t.Errorf("expected no password in the message, got %q", message)
			}
		})
	}
}
