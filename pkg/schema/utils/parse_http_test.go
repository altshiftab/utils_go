package utils

import (
	"bytes"
	"context"
	"fmt"
	"net"
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

func TestParseHttpKeptStartOfResponseBody(t *testing.T) {
	t.Parallel()

	gzipStart := []byte{0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03}

	testCases := []struct {
		name        string
		kept        []byte
		size        int
		wantContent string
		wantBytes   int
	}{
		{name: "a whole body", kept: []byte("0123456789"), size: 0, wantContent: "0123456789", wantBytes: 10},
		{name: "text of which only the start was kept", kept: []byte("0123456789"), size: 1000, wantContent: "0123456789... [truncated, 1000 bytes in full]", wantBytes: 1000},
		{name: "binary of which only the start was kept", kept: gzipStart, size: 5000, wantContent: "[binary, 5000 bytes]", wantBytes: 5000},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			base, err := ParseHttpContextWithBodyLimit(
				&altshiftHttpTypes.HttpContext{
					Response:         &http.Response{StatusCode: http.StatusOK, Header: http.Header{}},
					ResponseBody:     testCase.kept,
					ResponseBodySize: testCase.size,
				},
				0,
			)
			if err != nil {
				t.Fatalf("parse http context: %v", err)
			}
			if base == nil || base.Http == nil || base.Http.Response == nil || base.Http.Response.Body == nil {
				t.Fatalf("expected a response body, got %+v", base)
			}

			body := base.Http.Response.Body
			if body.Content != testCase.wantContent {
				t.Errorf("content = %q, want %q", body.Content, testCase.wantContent)
			}
			if body.Bytes != testCase.wantBytes {
				t.Errorf("bytes = %d, want %d", body.Bytes, testCase.wantBytes)
			}
		})
	}
}

// forwardedRequest is a request from 203.0.113.9 to service.example.com, on a
// connection accepted at 10.0.0.2:8080, carrying forwarding headers a client
// could have written itself.
func forwardedRequest(t *testing.T, headers map[string]string) *http.Request {
	t.Helper()

	request, err := http.NewRequestWithContext(
		context.WithValue(t.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("10.0.0.2"), Port: 8080}),
		http.MethodGet,
		"http://service.example.com/x",
		nil,
	)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	request.RemoteAddr = "203.0.113.9:54321"
	request.Proto, request.ProtoMajor, request.ProtoMinor = "HTTP/1.1", 1, 1
	for name, value := range headers {
		request.Header.Set(name, value)
	}

	return request
}

// forgedHeaders are what a client writes to be logged as someone else.
var forgedHeaders = map[string]string{
	"X-Forwarded-For":     "192.0.2.1",
	"Forwarded":           "for=192.0.2.2",
	"X-Client-Ip-Address": "192.0.2.3",
}

func targetIpPort(target *schema.Target) string {
	if target == nil {
		return "<nil>"
	}

	return fmt.Sprintf("%s:%d", target.Ip, target.Port)
}

// ParseHttp is what a forward proxy logs a request through with, and the
// forwarding headers it sees are the client's own: none of them is believed.
func TestParseHttpIgnoresForwardingHeaders(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		headers map[string]string
	}{
		{name: "no forwarding headers", headers: nil},
		{name: "forged forwarding headers", headers: forgedHeaders},
		{name: "Forwarded alone", headers: map[string]string{"Forwarded": "for=192.0.2.2"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			base, err := ParseHttp(forwardedRequest(t, testCase.headers), nil, nil, nil)
			if err != nil {
				t.Fatalf("parse http: %v", err)
			}
			if base == nil {
				t.Fatal("nil base")
			}

			if got := targetIpPort(base.Source); got != "203.0.113.9:54321" {
				t.Errorf("source = %s, want 203.0.113.9:54321", got)
			}
			if got := targetIpPort(base.Client); got != "203.0.113.9:54321" {
				t.Errorf("client = %s, want the source", got)
			}
			if got := targetIpPort(base.Server); got != "10.0.0.2:8080" {
				t.Errorf("server = %s, want the destination", got)
			}
			if base.Network == nil || base.Network.ForwardedIp != "" {
				t.Errorf("network = %+v, want no forwarded ip", base.Network)
			}
		})
	}
}

// ParseHttpContext renders the client the mux resolved on arrival, and the
// connection as the proxy's.
func TestParseHttpContextClientAddress(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		headers            map[string]string
		clientAddress      string
		expectedClient     string
		expectedForwarded  string
		expectedServerPort int
	}{
		{
			name:               "a resolved client",
			headers:            forgedHeaders,
			clientAddress:      "198.51.100.7",
			expectedClient:     "198.51.100.7:0",
			expectedForwarded:  "198.51.100.7",
			expectedServerPort: 0,
		},
		{
			name:               "nothing resolved, forged headers",
			headers:            forgedHeaders,
			expectedClient:     "203.0.113.9:54321",
			expectedServerPort: 8080,
		},
		{
			name:               "nothing resolved, Forwarded alone",
			headers:            map[string]string{"Forwarded": "for=192.0.2.2"},
			expectedClient:     "203.0.113.9:54321",
			expectedServerPort: 8080,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			base, err := ParseHttpContext(&altshiftHttpTypes.HttpContext{
				Request:       forwardedRequest(t, testCase.headers),
				ClientAddress: testCase.clientAddress,
			})
			if err != nil {
				t.Fatalf("parse http context: %v", err)
			}
			if base == nil {
				t.Fatal("nil base")
			}

			if got := targetIpPort(base.Client); got != testCase.expectedClient {
				t.Errorf("client = %s, want %s", got, testCase.expectedClient)
			}
			if base.Client != nil && testCase.clientAddress != "" && base.Client.Address != testCase.clientAddress {
				t.Errorf("client address = %q, want %q", base.Client.Address, testCase.clientAddress)
			}
			if got := targetIpPort(base.Source); got != "203.0.113.9:54321" {
				t.Errorf("source = %s, want the connection's peer", got)
			}
			if got := targetIpPort(base.Destination); got != "10.0.0.2:8080" {
				t.Errorf("destination = %s, want the connection's own", got)
			}
			if base.Network == nil || base.Network.ForwardedIp != testCase.expectedForwarded {
				t.Errorf("network = %+v, want forwarded ip %q", base.Network, testCase.expectedForwarded)
			}
			if base.Server == nil {
				t.Fatal("no server")
			}
			if base.Server.Port != testCase.expectedServerPort {
				t.Errorf("server port = %d, want %d", base.Server.Port, testCase.expectedServerPort)
			}
			if base.Server.Domain != "service.example.com" {
				t.Errorf("server domain = %q, want service.example.com", base.Server.Domain)
			}
			if !strings.HasPrefix(base.Message, strings.Split(testCase.expectedClient, ":")[0]+" ") {
				t.Errorf("message = %q, want it to begin with the client", base.Message)
			}
		})
	}
}
