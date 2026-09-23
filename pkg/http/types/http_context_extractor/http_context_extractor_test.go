package http_context_extractor

import (
	"bufio"
	"context"
	"encoding/base64"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	altshiftHttpContext "github.com/altshiftab/utils_go/pkg/http/context"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	csp "github.com/altshiftab/utils_go/pkg/http/types/content_security_policy"
	"github.com/altshiftab/utils_go/pkg/http/types/http_context_extractor/http_context_extractor_config"
	"github.com/altshiftab/utils_go/pkg/http/types/integrity_policy"
	"github.com/altshiftab/utils_go/pkg/http/types/reporting_api"
	altshiftSchemaTypes "github.com/altshiftab/utils_go/pkg/schema"
)

func TestMaskJws(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "valid JWS compact serialization",
			input: "header.payload.signature",
			want:  "header.payload.(MASKED)",
		},
		{
			name:  "valid JWS with realistic base64 parts",
			input: "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc123signature",
			want:  "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.(MASKED)",
		},
		{
			name:  "invalid JWS, not enough parts",
			input: "nodelimiters",
			want:  "(MASKED)",
		},
		{
			name:  "invalid JWS, only two parts",
			input: "header.payload",
			want:  "(MASKED)",
		},
		{
			name:  "empty string",
			input: "",
			want:  "(MASKED)",
		},
		{
			name:  "JWS with dots in signature",
			input: "a.b.c.d",
			want:  "a.b.(MASKED)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := maskJws(tc.input)
			if got != tc.want {
				t.Errorf("maskJws(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestMaskBasicAuth(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "valid basic auth credentials",
			input: base64.StdEncoding.EncodeToString([]byte("user:password")),
			want:  base64.StdEncoding.EncodeToString([]byte("user:")) + "(MASKED)",
		},
		{
			name:  "valid basic auth with empty password",
			input: base64.StdEncoding.EncodeToString([]byte("user:")),
			want:  base64.StdEncoding.EncodeToString([]byte("user:")) + "(MASKED)",
		},
		{
			name:  "valid basic auth with colon in password",
			input: base64.StdEncoding.EncodeToString([]byte("user:pass:word")),
			want:  base64.StdEncoding.EncodeToString([]byte("user:")) + "(MASKED)",
		},
		{
			name:  "invalid base64",
			input: "not-valid-base64!!!",
			want:  "(MASKED)",
		},
		{
			name:  "valid base64 but no colon separator",
			input: base64.StdEncoding.EncodeToString([]byte("nocolon")),
			want:  "(MASKED)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := maskBasicAuth(tc.input)
			if got != tc.want {
				t.Errorf("maskBasicAuth(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestMaskSetCookieHeader(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "cookie with JWS value",
			input: "session=header.payload.signature; Path=/; HttpOnly",
			want:  "session=header.payload.(MASKED); Path=/; HttpOnly",
		},
		{
			name:  "cookie with non-JWS value",
			input: "session=simplevalue; Path=/",
			want:  "session=(MASKED); Path=/",
		},
		{
			name:  "empty cookie value",
			input: "session=; Path=/",
			want:  "session=(MASKED); Path=/",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := maskSetCookieHeader(tc.input)
			if got != tc.want {
				t.Errorf("maskSetCookieHeader(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestMaskCookieHeader(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single cookie with JWS value",
			input: "session=header.payload.signature",
			want:  "session=header.payload.(MASKED)",
		},
		{
			name:  "multiple cookies",
			input: "a=x.y.z; b=p.q.r",
			want:  "a=x.y.(MASKED); b=p.q.(MASKED)",
		},
		{
			name:  "single cookie with non-JWS value",
			input: "session=simplevalue",
			want:  "session=(MASKED)",
		},
		{
			name:  "empty cookie header",
			input: "",
			want:  "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := maskCookieHeader(tc.input)
			if got != tc.want {
				t.Errorf("maskCookieHeader(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestExtractNormalizedHeaders(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		host       string
		header     http.Header
		wantParts  []string // substrings that must appear
		avoidParts []string // substrings that must NOT appear
	}{
		{
			name:      "plain header",
			header:    http.Header{"X-Custom": {"value1"}},
			wantParts: []string{"X-Custom: value1\r\n"},
		},
		{
			name:      "Set-Cookie header with JWS",
			header:    http.Header{"Set-Cookie": {"token=a.b.c; Path=/; HttpOnly"}},
			wantParts: []string{"Set-Cookie: token=a.b.(MASKED); Path=/; HttpOnly\r\n"},
		},
		{
			name:      "Cookie header with JWS",
			header:    http.Header{"Cookie": {"sess=x.y.z"}},
			wantParts: []string{"Cookie: sess=x.y.(MASKED)\r\n"},
		},
		{
			name:       "X-Goog-Iap-Jwt-Assertion masked",
			header:     http.Header{"X-Goog-Iap-Jwt-Assertion": {"header.payload.signature"}},
			wantParts:  []string{"X-Goog-Iap-Jwt-Assertion: header.payload.(MASKED)\r\n"},
			avoidParts: []string{"signature"},
		},
		{
			name: "Authorization Bearer with JWS",
			header: http.Header{
				"Authorization": {"Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIn0.Q70dVMtrOQzEFmGOxPAKbNOUSQMISCLhEDfGpMG0WM4"},
			},
			wantParts:  []string{"Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIn0.(MASKED)\r\n"},
			avoidParts: []string{"Q70dVMtrOQzEFmGOxPAKbNOUSQMISCLhEDfGpMG0WM4"},
		},
		{
			name: "Authorization Basic",
			header: http.Header{
				"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))},
			},
			wantParts:  []string{"Authorization:"},
			avoidParts: []string{"pass"},
		},
		{
			name:      "empty header",
			header:    http.Header{},
			wantParts: nil,
		},
		{
			name:      "host prepended",
			host:      "example.com",
			header:    http.Header{"X-Custom": {"value1"}},
			wantParts: []string{"Host: example.com\r\n", "X-Custom: value1\r\n"},
		},
		{
			name:       "empty host is omitted",
			host:       "",
			header:     http.Header{"X-Custom": {"value1"}},
			wantParts:  []string{"X-Custom: value1\r\n"},
			avoidParts: []string{"Host:"},
		},
		{
			name:      "host with empty header",
			host:      "example.com",
			header:    http.Header{},
			wantParts: []string{"Host: example.com\r\n"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractNormalizedHeaders(tc.host, tc.header, nil)

			for _, part := range tc.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("extractNormalizedHeaders() = %q, want substring %q", got, part)
				}
			}

			for _, avoid := range tc.avoidParts {
				if strings.Contains(got, avoid) {
					t.Errorf("extractNormalizedHeaders() = %q, should not contain %q", got, avoid)
				}
			}
		})
	}
}

func TestExtractor_Handle_NilRecord(t *testing.T) {
	t.Parallel()
	e := &Extractor{}
	err := e.Handle(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error for nil record, got %v", err)
	}
}

func TestExtractor_Handle_RequestId(t *testing.T) {
	t.Parallel()
	e := &Extractor{}
	ctx := context.WithValue(context.Background(), altshiftHttpContext.RequestIdContextKey, "test-request-id")
	record := &slog.Record{}

	err := e.Handle(ctx, record)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the record has the request id attribute.
	found := false
	record.Attrs(func(a slog.Attr) bool {
		if a.Key == "http" {
			a.Value.Resolve()
			for _, inner := range a.Value.Group() {
				if inner.Key == "request" {
					for _, attr := range inner.Value.Group() {
						if attr.Key == "id" && attr.Value.String() == "test-request-id" {
							found = true
							return false
						}
					}
				}
			}
		}
		return true
	})
	if !found {
		t.Error("expected http.request.id attribute in record")
	}
}

func TestExtractor_Handle_NoContext(t *testing.T) {
	t.Parallel()
	e := &Extractor{}
	record := &slog.Record{}

	err := e.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractor_Handle_HttpContext(t *testing.T) {
	t.Parallel()

	req, err := http.ReadRequest(bufio.NewReader(strings.NewReader(
		"GET /test HTTP/1.1\r\nHost: example.com\r\nX-Custom: value\r\n\r\n",
	)))
	if err != nil {
		t.Fatalf("read request: %v", err)
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{"Content-Type": {"text/plain"}},
	}

	httpContext := &altshiftHttpTypes.HttpContext{
		Request:  req,
		Response: resp,
	}

	ctx := context.WithValue(context.Background(), altshiftHttpContext.HttpContextContextKey, httpContext)
	record := &slog.Record{}

	e := &Extractor{}
	err = e.Handle(ctx, record)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the record has attributes added from the HTTP context.
	hasAttrs := false
	record.Attrs(func(a slog.Attr) bool {
		hasAttrs = true
		return true
	})
	if !hasAttrs {
		t.Error("expected attributes to be added from HTTP context")
	}
}

func TestExtractor_Handle_HttpContextWithHeaders(t *testing.T) {
	t.Parallel()

	req, err := http.ReadRequest(bufio.NewReader(strings.NewReader(
		"GET /test HTTP/1.1\r\nHost: example.com\r\nCookie: sess=a.b.c\r\n\r\n",
	)))
	if err != nil {
		t.Fatalf("read request: %v", err)
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{"Set-Cookie": {"token=x.y.z; Path=/"}},
	}

	httpContext := &altshiftHttpTypes.HttpContext{
		Request:  req,
		Response: resp,
	}

	ctx := context.WithValue(context.Background(), altshiftHttpContext.HttpContextContextKey, httpContext)
	record := &slog.Record{}

	e := &Extractor{}
	err = e.Handle(ctx, record)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractor_Handle_HttpContextNilRequest(t *testing.T) {
	t.Parallel()

	httpContext := &altshiftHttpTypes.HttpContext{
		Request:  nil,
		Response: nil,
	}

	ctx := context.WithValue(context.Background(), altshiftHttpContext.HttpContextContextKey, httpContext)
	record := &slog.Record{}

	e := &Extractor{}
	err := e.Handle(ctx, record)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractor_Handle_HttpContextRequestNoHeaders(t *testing.T) {
	t.Parallel()

	req, err := http.ReadRequest(bufio.NewReader(strings.NewReader(
		"GET /test HTTP/1.1\r\nHost: example.com\r\n\r\n",
	)))
	if err != nil {
		t.Fatalf("read request: %v", err)
	}

	httpContext := &altshiftHttpTypes.HttpContext{
		Request: req,
	}

	ctx := context.WithValue(context.Background(), altshiftHttpContext.HttpContextContextKey, httpContext)
	record := &slog.Record{}

	e := &Extractor{}
	err = e.Handle(ctx, record)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractor_Handle_MessageNotOverwrittenWithoutRequest(t *testing.T) {
	t.Parallel()

	e := &Extractor{}
	record := &slog.Record{}
	record.Message = "original message"

	err := e.Handle(context.Background(), record)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if record.Message != "original message" {
		t.Errorf("record.Message = %q, want %q", record.Message, "original message")
	}
}

func TestExtractUnverifiedUser_AuthorizationBearer_SessionToken(t *testing.T) {
	t.Parallel()

	// JWT with sub="user123:user@example.com", azp="tenant1:MyTenant", roles=["admin"]
	serialization := "eyJhbGciOiAiSFMyNTYifQ.eyJzdWIiOiAidXNlcjEyMzp1c2VyQGV4YW1wbGUuY29tIiwgImF6cCI6ICJ0ZW5hbnQxOk15VGVuYW50IiwgInJvbGVzIjogWyJhZG1pbiJdLCAiaXNzIjogInRlc3QiLCAiYXVkIjogInRlc3QifQ.fakesig"
	header := http.Header{"Authorization": {"Bearer " + serialization}}

	user := extractUnverifiedUser(header)
	if user == nil {
		t.Fatal("expected non-nil user")
	}
	if !user.Unverified {
		t.Error("expected Unverified to be true")
	}
	if user.Id != "user123" {
		t.Errorf("user.Id = %q, want %q", user.Id, "user123")
	}
	if user.Email != "user@example.com" {
		t.Errorf("user.Email = %q, want %q", user.Email, "user@example.com")
	}
	if user.Group == nil || user.Group.Id != "tenant1" || user.Group.Name != "MyTenant" {
		t.Errorf("user.Group = %+v, want {Id: tenant1, Name: MyTenant}", user.Group)
	}
	if len(user.Roles) != 1 || user.Roles[0] != "admin" {
		t.Errorf("user.Roles = %v, want [admin]", user.Roles)
	}
}

func TestExtractUnverifiedUser_Cookie_EmailSub(t *testing.T) {
	t.Parallel()

	// JWT with sub="user@example.com" (no colon, so falls back to sub claim)
	serialization := "eyJhbGciOiAiSFMyNTYifQ.eyJzdWIiOiAidXNlckBleGFtcGxlLmNvbSJ9.fakesig"
	header := http.Header{"Cookie": {"session=" + serialization}}

	user := extractUnverifiedUser(header)
	if user == nil {
		t.Fatal("expected non-nil user")
	}
	if !user.Unverified {
		t.Error("expected Unverified to be true")
	}
	if user.Email != "user@example.com" {
		t.Errorf("user.Email = %q, want %q", user.Email, "user@example.com")
	}
	if user.Name != "" {
		t.Errorf("user.Name = %q, want empty", user.Name)
	}
}

func TestExtractUnverifiedUser_Cookie_NameSub(t *testing.T) {
	t.Parallel()

	// JWT with sub="johndoe"
	serialization := "eyJhbGciOiAiSFMyNTYifQ.eyJzdWIiOiAiam9obmRvZSJ9.fakesig"
	header := http.Header{"Cookie": {"session=" + serialization}}

	user := extractUnverifiedUser(header)
	if user == nil {
		t.Fatal("expected non-nil user")
	}
	if !user.Unverified {
		t.Error("expected Unverified to be true")
	}
	if user.Name != "johndoe" {
		t.Errorf("user.Name = %q, want %q", user.Name, "johndoe")
	}
	if user.Email != "" {
		t.Errorf("user.Email = %q, want empty", user.Email)
	}
}

func TestExtractUnverifiedUser_BasicAuth_Email(t *testing.T) {
	t.Parallel()

	credentials := base64.StdEncoding.EncodeToString([]byte("user@example.com:password"))
	header := http.Header{"Authorization": {"Basic " + credentials}}

	user := extractUnverifiedUser(header)
	if user == nil {
		t.Fatal("expected non-nil user")
	}
	if !user.Unverified {
		t.Error("expected Unverified to be true")
	}
	if user.Email != "user@example.com" {
		t.Errorf("user.Email = %q, want %q", user.Email, "user@example.com")
	}
	if user.Name != "" {
		t.Errorf("user.Name = %q, want empty", user.Name)
	}
}

func TestExtractUnverifiedUser_BasicAuth_Username(t *testing.T) {
	t.Parallel()

	credentials := base64.StdEncoding.EncodeToString([]byte("johndoe:password"))
	header := http.Header{"Authorization": {"Basic " + credentials}}

	user := extractUnverifiedUser(header)
	if user == nil {
		t.Fatal("expected non-nil user")
	}
	if !user.Unverified {
		t.Error("expected Unverified to be true")
	}
	if user.Name != "johndoe" {
		t.Errorf("user.Name = %q, want %q", user.Name, "johndoe")
	}
	if user.Email != "" {
		t.Errorf("user.Email = %q, want empty", user.Email)
	}
}

func TestExtractUnverifiedUser_NoJwt(t *testing.T) {
	t.Parallel()

	header := http.Header{"X-Custom": {"value"}}
	user := extractUnverifiedUser(header)
	if user != nil {
		t.Errorf("expected nil user, got %+v", user)
	}
}

func TestExtractUnverifiedUser_InvalidJwt(t *testing.T) {
	t.Parallel()

	header := http.Header{"Authorization": {"Bearer not-a-jwt"}}
	user := extractUnverifiedUser(header)
	if user != nil {
		t.Errorf("expected nil user, got %+v", user)
	}
}

func TestExtractor_Handle_UserNotOverwritten(t *testing.T) {
	t.Parallel()

	serialization := "eyJhbGciOiAiSFMyNTYifQ.eyJzdWIiOiAiam9obmRvZSJ9.fakesig"
	req, err := http.ReadRequest(bufio.NewReader(strings.NewReader(
		"GET /test HTTP/1.1\r\nHost: example.com\r\nAuthorization: Bearer " + serialization + "\r\n\r\n",
	)))
	if err != nil {
		t.Fatalf("read request: %v", err)
	}

	existingUser := &altshiftSchemaTypes.User{Name: "existing"}
	httpContext := &altshiftHttpTypes.HttpContext{
		Request: req,
		User:    existingUser,
	}

	ctx := context.WithValue(context.Background(), altshiftHttpContext.HttpContextContextKey, httpContext)
	record := &slog.Record{}

	e := &Extractor{}
	if err := e.Handle(ctx, record); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if httpContext.User != existingUser {
		t.Error("expected existing user to not be overwritten")
	}
}

func TestExtractor_Handle_HttpContextResponseNoHeaders(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     nil,
	}

	httpContext := &altshiftHttpTypes.HttpContext{
		Response: resp,
	}

	ctx := context.WithValue(context.Background(), altshiftHttpContext.HttpContextContextKey, httpContext)
	record := &slog.Record{}

	e := &Extractor{}
	err := e.Handle(ctx, record)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractor_MaskUrl(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		maskedUrlParams []*altshiftSchemaTypes.Url
		inputUrl        *altshiftSchemaTypes.Url
		expectedUrl     *altshiftSchemaTypes.Url
	}{
		{
			name:            "mask single query parameter, query-only pattern",
			maskedUrlParams: []*altshiftSchemaTypes.Url{{Query: "apikey"}},
			inputUrl: &altshiftSchemaTypes.Url{
				Full:     "https://example.com/api?apikey=secret123&user=john",
				Original: "https://example.com/api?apikey=secret123&user=john",
				Query:    "apikey=secret123&user=john",
			},
			expectedUrl: &altshiftSchemaTypes.Url{
				Full:     "https://example.com/api?apikey=%28MASKED%29&user=john",
				Original: "https://example.com/api?apikey=%28MASKED%29&user=john",
				Query:    "apikey=%28MASKED%29&user=john",
			},
		},
		{
			name:            "mask multiple query parameters from one pattern",
			maskedUrlParams: []*altshiftSchemaTypes.Url{{Query: "apikey&token&password"}},
			inputUrl: &altshiftSchemaTypes.Url{
				Full:     "https://example.com/api?apikey=secret123&token=abc456&user=john&password=pass123",
				Original: "https://example.com/api?apikey=secret123&token=abc456&user=john&password=pass123",
				Query:    "apikey=secret123&token=abc456&user=john&password=pass123",
			},
			expectedUrl: &altshiftSchemaTypes.Url{
				Full:     "https://example.com/api?apikey=%28MASKED%29&password=%28MASKED%29&token=%28MASKED%29&user=john",
				Original: "https://example.com/api?apikey=%28MASKED%29&password=%28MASKED%29&token=%28MASKED%29&user=john",
				Query:    "apikey=%28MASKED%29&password=%28MASKED%29&token=%28MASKED%29&user=john",
			},
		},
		{
			name:            "no parameters to mask, param not in incoming URL",
			maskedUrlParams: []*altshiftSchemaTypes.Url{{Query: "notfound"}},
			inputUrl: &altshiftSchemaTypes.Url{
				Full:     "https://example.com/api?user=john&page=1",
				Original: "https://example.com/api?user=john&page=1",
				Query:    "user=john&page=1",
			},
			expectedUrl: &altshiftSchemaTypes.Url{
				Full:     "https://example.com/api?user=john&page=1",
				Original: "https://example.com/api?user=john&page=1",
				Query:    "user=john&page=1",
			},
		},
		{
			name:            "empty URL fields",
			maskedUrlParams: []*altshiftSchemaTypes.Url{{Query: "apikey"}},
			inputUrl: &altshiftSchemaTypes.Url{
				Full:     "",
				Original: "",
				Query:    "",
			},
			expectedUrl: &altshiftSchemaTypes.Url{
				Full:     "",
				Original: "",
				Query:    "",
			},
		},
		{
			name:            "nil maskedUrlParams",
			maskedUrlParams: nil,
			inputUrl: &altshiftSchemaTypes.Url{
				Full:     "https://example.com/api?apikey=secret123",
				Original: "https://example.com/api?apikey=secret123",
				Query:    "apikey=secret123",
			},
			expectedUrl: &altshiftSchemaTypes.Url{
				Full:     "https://example.com/api?apikey=secret123",
				Original: "https://example.com/api?apikey=secret123",
				Query:    "apikey=secret123",
			},
		},
		{
			name:            "invalid URL format",
			maskedUrlParams: []*altshiftSchemaTypes.Url{{Query: "apikey"}},
			inputUrl: &altshiftSchemaTypes.Url{
				Full:     "not-a-valid-url",
				Original: "not-a-valid-url",
				Query:    "apikey=secret123",
			},
			expectedUrl: &altshiftSchemaTypes.Url{
				Full:     "not-a-valid-url",
				Original: "not-a-valid-url",
				Query:    "apikey=%28MASKED%29",
			},
		},
		{
			name:            "query string only",
			maskedUrlParams: []*altshiftSchemaTypes.Url{{Query: "token&auth"}},
			inputUrl: &altshiftSchemaTypes.Url{
				Full:     "",
				Original: "",
				Query:    "token=abc123&auth=xyz789&public=data",
			},
			expectedUrl: &altshiftSchemaTypes.Url{
				Full:     "",
				Original: "",
				Query:    "auth=%28MASKED%29&public=data&token=%28MASKED%29",
			},
		},
		{
			name:            "pattern with path match",
			maskedUrlParams: []*altshiftSchemaTypes.Url{{Path: "/api", Query: "apikey"}},
			inputUrl: &altshiftSchemaTypes.Url{
				Path:     "/api",
				Full:     "https://example.com/api?apikey=secret123&user=john",
				Original: "/api?apikey=secret123&user=john",
				Query:    "apikey=secret123&user=john",
			},
			expectedUrl: &altshiftSchemaTypes.Url{
				Path:     "/api",
				Full:     "https://example.com/api?apikey=%28MASKED%29&user=john",
				Original: "/api?apikey=%28MASKED%29&user=john",
				Query:    "apikey=%28MASKED%29&user=john",
			},
		},
		{
			name:            "pattern with path mismatch",
			maskedUrlParams: []*altshiftSchemaTypes.Url{{Path: "/other", Query: "apikey"}},
			inputUrl: &altshiftSchemaTypes.Url{
				Path:     "/api",
				Full:     "https://example.com/api?apikey=secret123&user=john",
				Original: "/api?apikey=secret123&user=john",
				Query:    "apikey=secret123&user=john",
			},
			expectedUrl: &altshiftSchemaTypes.Url{
				Path:     "/api",
				Full:     "https://example.com/api?apikey=secret123&user=john",
				Original: "/api?apikey=secret123&user=john",
				Query:    "apikey=secret123&user=john",
			},
		},
		{
			name:            "pattern with domain match",
			maskedUrlParams: []*altshiftSchemaTypes.Url{{Domain: "example.com", Query: "token"}},
			inputUrl: &altshiftSchemaTypes.Url{
				Domain:   "example.com",
				Full:     "https://example.com/api?token=secret",
				Original: "/api?token=secret",
				Query:    "token=secret",
			},
			expectedUrl: &altshiftSchemaTypes.Url{
				Domain:   "example.com",
				Full:     "https://example.com/api?token=%28MASKED%29",
				Original: "/api?token=%28MASKED%29",
				Query:    "token=%28MASKED%29",
			},
		},
		{
			name:            "pattern with domain mismatch",
			maskedUrlParams: []*altshiftSchemaTypes.Url{{Domain: "other.com", Query: "token"}},
			inputUrl: &altshiftSchemaTypes.Url{
				Domain:   "example.com",
				Full:     "https://example.com/api?token=secret",
				Original: "/api?token=secret",
				Query:    "token=secret",
			},
			expectedUrl: &altshiftSchemaTypes.Url{
				Domain:   "example.com",
				Full:     "https://example.com/api?token=secret",
				Original: "/api?token=secret",
				Query:    "token=secret",
			},
		},
		{
			name: "multiple patterns, both matching",
			maskedUrlParams: []*altshiftSchemaTypes.Url{
				{Query: "apikey"},
				{Path: "/api", Query: "token"},
			},
			inputUrl: &altshiftSchemaTypes.Url{
				Path:     "/api",
				Full:     "https://example.com/api?apikey=secret&token=abc",
				Original: "/api?apikey=secret&token=abc",
				Query:    "apikey=secret&token=abc",
			},
			expectedUrl: &altshiftSchemaTypes.Url{
				Path:     "/api",
				Full:     "https://example.com/api?apikey=%28MASKED%29&token=%28MASKED%29",
				Original: "/api?apikey=%28MASKED%29&token=%28MASKED%29",
				Query:    "apikey=%28MASKED%29&token=%28MASKED%29",
			},
		},
		{
			name: "multiple patterns, only one matching",
			maskedUrlParams: []*altshiftSchemaTypes.Url{
				{Path: "/other", Query: "apikey"},
				{Query: "token"},
			},
			inputUrl: &altshiftSchemaTypes.Url{
				Path:     "/api",
				Full:     "https://example.com/api?apikey=secret&token=abc",
				Original: "/api?apikey=secret&token=abc",
				Query:    "apikey=secret&token=abc",
			},
			expectedUrl: &altshiftSchemaTypes.Url{
				Path:     "/api",
				Full:     "https://example.com/api?apikey=secret&token=%28MASKED%29",
				Original: "/api?apikey=secret&token=%28MASKED%29",
				Query:    "apikey=secret&token=%28MASKED%29",
			},
		},
		{
			name:            "pattern with scheme and port match",
			maskedUrlParams: []*altshiftSchemaTypes.Url{{Scheme: "https", Port: 443, Query: "secret"}},
			inputUrl: &altshiftSchemaTypes.Url{
				Scheme:   "https",
				Port:     443,
				Full:     "https://example.com:443/api?secret=val",
				Original: "/api?secret=val",
				Query:    "secret=val",
			},
			expectedUrl: &altshiftSchemaTypes.Url{
				Scheme:   "https",
				Port:     443,
				Full:     "https://example.com:443/api?secret=%28MASKED%29",
				Original: "/api?secret=%28MASKED%29",
				Query:    "secret=%28MASKED%29",
			},
		},
		{
			name:            "pattern with no query is a no-op",
			maskedUrlParams: []*altshiftSchemaTypes.Url{{Path: "/api"}},
			inputUrl: &altshiftSchemaTypes.Url{
				Path:     "/api",
				Full:     "https://example.com/api?apikey=secret",
				Original: "/api?apikey=secret",
				Query:    "apikey=secret",
			},
			expectedUrl: &altshiftSchemaTypes.Url{
				Path:     "/api",
				Full:     "https://example.com/api?apikey=secret",
				Original: "/api?apikey=secret",
				Query:    "apikey=secret",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e := &Extractor{
				MaskedUrlParams: tc.maskedUrlParams,
			}

			urlCopy := *tc.inputUrl

			e.maskUrl(&urlCopy)

			if urlCopy.Full != tc.expectedUrl.Full {
				t.Errorf("Full URL mismatch:\ngot:  %q\nwant: %q", urlCopy.Full, tc.expectedUrl.Full)
			}
			if urlCopy.Original != tc.expectedUrl.Original {
				t.Errorf("Original URL mismatch:\ngot:  %q\nwant: %q", urlCopy.Original, tc.expectedUrl.Original)
			}
			if urlCopy.Query != tc.expectedUrl.Query {
				t.Errorf("Query string mismatch:\ngot:  %q\nwant: %q", urlCopy.Query, tc.expectedUrl.Query)
			}
		})
	}
}

func TestExtractor_MaskUrl_NilUrl(t *testing.T) {
	t.Parallel()

	e := &Extractor{
		MaskedUrlParams: []*altshiftSchemaTypes.Url{{Query: "apikey"}},
	}

	// Should not panic with nil URL
	e.maskUrl(nil)
}

func TestPathMatches(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		pattern  string
		incoming string
		want     bool
	}{
		{name: "empty pattern matches anything", pattern: "", incoming: "/a/b", want: true},
		{name: "exact match", pattern: "/a/b", incoming: "/a/b", want: true},
		{name: "exact mismatch", pattern: "/a/b", incoming: "/a/c", want: false},
		{name: "exact pattern rejects child", pattern: "/a/b", incoming: "/a/b/c", want: false},
		{name: "prefix matches child", pattern: "/a/b/*", incoming: "/a/b/c", want: true},
		{name: "prefix matches grandchild", pattern: "/a/b/*", incoming: "/a/b/c/d", want: true},
		{name: "prefix does not match exact prefix without slash", pattern: "/a/b/*", incoming: "/a/b", want: false},
		{name: "prefix does not match sibling", pattern: "/a/b/*", incoming: "/a/bc/d", want: false},
		{name: "prefix root /*", pattern: "/*", incoming: "/anything", want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := pathMatches(tc.pattern, tc.incoming); got != tc.want {
				t.Errorf("pathMatches(%q, %q) = %v, want %v", tc.pattern, tc.incoming, got, tc.want)
			}
		})
	}
}

func TestUrlMatchesPattern_PathPrefix(t *testing.T) {
	t.Parallel()

	// A pattern with /* matches incoming requests under that segment prefix
	// and surfaces the masked query parameters as usual.
	pattern := &altshiftSchemaTypes.Url{
		Domain: "example.com",
		Path:   "/api/v1/profiles/*",
		Query:  "secret",
	}

	cases := []struct {
		name         string
		incoming     *altshiftSchemaTypes.Url
		wantMatch    bool
		wantParamLen int
	}{
		{
			name: "child path with query",
			incoming: &altshiftSchemaTypes.Url{
				Domain: "example.com",
				Path:   "/api/v1/profiles/123",
				Query:  "secret=abc&other=ok",
			},
			wantMatch:    true,
			wantParamLen: 1,
		},
		{
			name: "exact prefix without slash does not match",
			incoming: &altshiftSchemaTypes.Url{
				Domain: "example.com",
				Path:   "/api/v1/profiles",
				Query:  "secret=abc",
			},
			wantMatch:    false,
			wantParamLen: 0,
		},
		{
			name: "sibling path does not match",
			incoming: &altshiftSchemaTypes.Url{
				Domain: "example.com",
				Path:   "/api/v1/profiles-v2/123",
				Query:  "secret=abc",
			},
			wantMatch:    false,
			wantParamLen: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotMatch, gotParams, _ := urlMatchesPattern(pattern, tc.incoming)
			if gotMatch != tc.wantMatch {
				t.Errorf("match: got %v, want %v", gotMatch, tc.wantMatch)
			}
			if len(gotParams) != tc.wantParamLen {
				t.Errorf("param count: got %d, want %d (params=%v)", len(gotParams), tc.wantParamLen, gotParams)
			}
		})
	}
}

// requestBodyContent walks a record's attributes for http.request.body.content.
func requestBodyContent(t *testing.T, record *slog.Record) (string, bool) {
	t.Helper()
	var (
		content string
		found   bool
	)
	record.Attrs(func(a slog.Attr) bool {
		if a.Key != "http" {
			return true
		}
		a.Value.Resolve()
		for _, request := range a.Value.Group() {
			if request.Key != "request" {
				continue
			}
			for _, body := range request.Value.Group() {
				if body.Key != "body" {
					continue
				}
				for _, field := range body.Value.Group() {
					if field.Key == "content" {
						content = field.Value.String()
						found = true
						return false
					}
				}
			}
		}
		return true
	})
	return content, found
}

func TestExtractor_Handle_MaskedRequestBody(t *testing.T) {
	t.Parallel()

	const secret = "client_secret=GOCSPX-supersecret&grant_type=refresh_token&refresh_token=1//0verysecret"

	newContext := func() context.Context {
		req, err := http.ReadRequest(bufio.NewReader(strings.NewReader(
			"POST /token HTTP/1.1\r\nHost: oauth2.example.com\r\nContent-Type: application/x-www-form-urlencoded\r\n\r\n",
		)))
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		httpContext := &altshiftHttpTypes.HttpContext{
			Request:     req,
			RequestBody: []byte(secret),
		}
		return context.WithValue(context.Background(), altshiftHttpContext.HttpContextContextKey, httpContext)
	}

	testCases := []struct {
		name     string
		patterns []*altshiftSchemaTypes.Url
		want     string
	}{
		{
			name:     "masked when path matches",
			patterns: []*altshiftSchemaTypes.Url{{Path: "/token"}},
			want:     maskedValue,
		},
		{
			name:     "masked when domain and path match",
			patterns: []*altshiftSchemaTypes.Url{{Domain: "oauth2.example.com", Path: "/token"}},
			want:     maskedValue,
		},
		{
			name:     "unmasked when no patterns",
			patterns: nil,
			want:     secret,
		},
		{
			name:     "unmasked when path does not match",
			patterns: []*altshiftSchemaTypes.Url{{Path: "/other"}},
			want:     secret,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := &Extractor{MaskedRequestBodyUrls: tc.patterns}
			record := &slog.Record{}
			if err := e.Handle(newContext(), record); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, found := requestBodyContent(t, record)
			if !found {
				t.Fatal("http.request.body.content attribute not found in record")
			}
			if got != tc.want {
				t.Errorf("request body content: got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestExtractor_Handle_RequestIdWrongType verifies that a request id that is not a string is left
// alone rather than added as one.
func TestExtractor_Handle_RequestIdWrongType(t *testing.T) {
	t.Parallel()

	e := &Extractor{}
	ctx := context.WithValue(context.Background(), altshiftHttpContext.RequestIdContextKey, 42)
	record := &slog.Record{}

	if err := e.Handle(ctx, record); err != nil {
		t.Fatalf("handle: %v", err)
	}

	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "http" {
			t.Errorf("an http attribute was added for a request id that is not a string: %v", attr)
		}
		return true
	})
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("without options", func(t *testing.T) {
		t.Parallel()

		extractor := New()
		if extractor == nil {
			t.Fatal("nil extractor")
		}

		// Nothing is masked and no message is replaced unless it is asked for.
		if extractor.ReplaceableMessages != nil {
			t.Errorf("replaceable messages: got %v, want none", extractor.ReplaceableMessages)
		}
		if extractor.MaskedUrlParams != nil || extractor.MaskedHeaders != nil {
			t.Errorf("masking: got %+v, %+v, want none", extractor.MaskedUrlParams, extractor.MaskedHeaders)
		}
	})

	t.Run("with replaceable messages", func(t *testing.T) {
		t.Parallel()

		extractor := New(http_context_extractor_config.WithReplaceableMessages("a message", "another"))
		if extractor == nil {
			t.Fatal("nil extractor")
		}

		// The messages are held as a set, being matched against one record message at a time.
		if len(extractor.ReplaceableMessages) != 2 {
			t.Fatalf("replaceable messages: got %v, want two", extractor.ReplaceableMessages)
		}
		if _, found := extractor.ReplaceableMessages["a message"]; !found {
			t.Errorf("replaceable messages: got %v", extractor.ReplaceableMessages)
		}
	})
}

// The token stands in for a credential carried in a query parameter -- the address of a page that
// is itself the proof of who is asking.
const maskingTestToken = "zhhjetavkELn1GOhywaXHdOatMAmhMro6ksgHu2XP7o" //nolint:gosec // G101: a stand-in, not a token.

// verificationUrl is that page's address, as a browser reports it and as a Referer carries it.
const verificationUrl = "https://dd.example.com/verification?token=" + maskingTestToken

// tokenExtractor is configured the way a service declaring a secret query parameter configures it.
func tokenExtractor() *Extractor {
	return &Extractor{
		MaskedUrlParams: []*altshiftSchemaTypes.Url{{Path: "/verification", Query: "token"}},
	}
}

// recordText renders everything a record says -- its message and every attribute, however nested --
// so a test can ask whether a secret survived anywhere in it.
func recordText(t *testing.T, record *slog.Record) string {
	t.Helper()

	var builder strings.Builder
	builder.WriteString(record.Message)

	var writeAttr func(slog.Attr)
	writeAttr = func(attr slog.Attr) {
		value := attr.Value.Resolve()
		if value.Kind() == slog.KindGroup {
			for _, inner := range value.Group() {
				writeAttr(inner)
			}
			return
		}
		builder.WriteString(" ")
		builder.WriteString(value.String())
	}

	record.Attrs(func(attr slog.Attr) bool {
		writeAttr(attr)
		return true
	})

	return builder.String()
}

func TestExtractor_MaskUrlString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		extractor *Extractor
		raw       string
		want      string
	}{
		{
			name:      "a declared parameter is masked",
			extractor: tokenExtractor(),
			raw:       verificationUrl,
			want:      "https://dd.example.com/verification?token=%28MASKED%29",
		},
		{
			name:      "another page's query is left alone",
			extractor: tokenExtractor(),
			raw:       "https://dd.example.com/orders?token=" + maskingTestToken,
			want:      "https://dd.example.com/orders?token=" + maskingTestToken,
		},
		{
			name: "a pattern scoped by domain matches the reported host",
			extractor: &Extractor{MaskedUrlParams: []*altshiftSchemaTypes.Url{
				{Domain: "dd.example.com", Path: "/verification", Query: "token"},
			}},
			raw:  verificationUrl,
			want: "https://dd.example.com/verification?token=%28MASKED%29",
		},
		{
			name: "a pattern scoped by another domain does not",
			extractor: &Extractor{MaskedUrlParams: []*altshiftSchemaTypes.Url{
				{Domain: "other.example.com", Path: "/verification", Query: "token"},
			}},
			raw:  verificationUrl,
			want: verificationUrl,
		},
		{
			name: "a pattern scoped by registered domain matches",
			extractor: &Extractor{MaskedUrlParams: []*altshiftSchemaTypes.Url{
				{RegisteredDomain: "example.com", Path: "/verification", Query: "token"},
			}},
			raw:  verificationUrl,
			want: "https://dd.example.com/verification?token=%28MASKED%29",
		},
		{
			// What a violation report puts where a URL would go when what it blocked was not one.
			name:      "the inline sentinel is not a url",
			extractor: tokenExtractor(),
			raw:       "inline",
			want:      "inline",
		},
		{
			name:      "the eval sentinel is not a url",
			extractor: tokenExtractor(),
			raw:       "eval",
			want:      "eval",
		},
		{
			name:      "the trusted types sentinel is not a url",
			extractor: tokenExtractor(),
			raw:       "trusted-types-policy",
			want:      "trusted-types-policy",
		},
		{
			name:      "an empty string",
			extractor: tokenExtractor(),
			raw:       "",
			want:      "",
		},
		{
			name:      "a url with no query",
			extractor: tokenExtractor(),
			raw:       "https://dd.example.com/verification",
			want:      "https://dd.example.com/verification",
		},
		{
			name:      "an extractor declaring nothing masks nothing",
			extractor: &Extractor{},
			raw:       verificationUrl,
			want:      verificationUrl,
		},
		{
			// A stray percent sign in an unrelated parameter makes the query unreadable. The
			// declared one is still in there, and still opens the page, so the whole query goes
			// rather than none of it.
			name:      "a query that cannot be read is replaced whole",
			extractor: tokenExtractor(),
			raw:       verificationUrl + "&next=%zz",
			want:      "https://dd.example.com/verification?" + maskedValue,
		},
		{
			// The same, for the separator Go stopped accepting.
			name:      "a semicolon separated query is replaced whole",
			extractor: tokenExtractor(),
			raw:       verificationUrl + ";next=1",
			want:      "https://dd.example.com/verification?" + maskedValue,
		},
		{
			// Unreadable, but on a path nothing was declared secret for.
			name:      "an undeclared path keeps its unreadable query",
			extractor: tokenExtractor(),
			raw:       "https://dd.example.com/orders?token=" + maskingTestToken + "&next=%zz",
			want:      "https://dd.example.com/orders?token=" + maskingTestToken + "&next=%zz",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.extractor.MaskUrlString(testCase.raw); got != testCase.want {
				t.Errorf("MaskUrlString(%q) = %q, want %q", testCase.raw, got, testCase.want)
			}
		})
	}
}

// A report names the page it is about. Masking it must not reach back into the struct the endpoint
// attached to the request, which is logged a second time by the access line the mux writes.
func TestExtractor_MaskReporting_DoesNotMutateItsInput(t *testing.T) {
	t.Parallel()

	referrer := verificationUrl
	sourceFile := verificationUrl
	reporting := &altshiftSchemaTypes.HttpReporting{
		CspViolations: []*reporting_api.Report[*csp.CSPViolationReportBody]{
			{
				URL: verificationUrl,
				Body: &csp.CSPViolationReportBody{
					DocumentURL: verificationUrl,
					BlockedURL:  "trusted-types-policy",
					Referrer:    &referrer,
					SourceFile:  &sourceFile,
				},
			},
		},
		IntegrityViolations: []*reporting_api.Report[*integrity_policy.IntegrityViolationReportBody]{
			{
				URL:  verificationUrl,
				Body: &integrity_policy.IntegrityViolationReportBody{DocumentUrl: verificationUrl, BlockedUrl: verificationUrl},
			},
		},
		CspReport: &csp.ReportEnvelope{
			CspReport: &csp.Report{DocumentURI: verificationUrl, BlockedUri: verificationUrl, Referrer: &referrer},
		},
	}

	masked := tokenExtractor().maskReporting(reporting)

	if masked == reporting {
		t.Fatal("maskReporting returned its input rather than a copy")
	}

	// Every URL the copy names is masked.
	for _, got := range []string{
		masked.CspViolations[0].URL,
		masked.CspViolations[0].Body.DocumentURL,
		*masked.CspViolations[0].Body.Referrer,
		*masked.CspViolations[0].Body.SourceFile,
		masked.IntegrityViolations[0].URL,
		masked.IntegrityViolations[0].Body.DocumentUrl,
		masked.IntegrityViolations[0].Body.BlockedUrl,
		masked.CspReport.CspReport.DocumentURI,
		masked.CspReport.CspReport.BlockedUri,
		*masked.CspReport.CspReport.Referrer,
	} {
		if strings.Contains(got, maskingTestToken) {
			t.Errorf("a reported url kept the token: %q", got)
		}
	}

	// And the original is untouched, so whatever logs it next still logs what arrived.
	for _, got := range []string{
		reporting.CspViolations[0].URL,
		reporting.CspViolations[0].Body.DocumentURL,
		reporting.IntegrityViolations[0].Body.DocumentUrl,
		reporting.CspReport.CspReport.DocumentURI,
		referrer,
		sourceFile,
	} {
		if !strings.Contains(got, maskingTestToken) {
			t.Errorf("maskReporting mutated the reporting it was given: %q", got)
		}
	}

	// What was never a URL is left as the browser wrote it.
	if masked.CspViolations[0].Body.BlockedURL != "trusted-types-policy" {
		t.Errorf("blocked url = %q, want it unchanged", masked.CspViolations[0].Body.BlockedURL)
	}
}

func TestExtractor_MaskReporting_Empty(t *testing.T) {
	t.Parallel()

	if got := tokenExtractor().maskReporting(nil); got != nil {
		t.Errorf("maskReporting(nil) = %v, want nil", got)
	}

	// An extractor declaring nothing hands back exactly what it was given.
	reporting := &altshiftSchemaTypes.HttpReporting{}
	if got := (&Extractor{}).maskReporting(reporting); got != reporting {
		t.Error("an extractor declaring no masks should return its input unchanged")
	}
}

// The page hands its address to everything it goes on to request, as the Referer, and that is a
// request of its own with a log entry of its own.
func TestExtractor_Handle_MasksReferer(t *testing.T) {
	t.Parallel()

	request, err := http.ReadRequest(bufio.NewReader(strings.NewReader(
		"GET /api/order/consented HTTP/1.1\r\nHost: dd.example.com\r\nReferer: " + verificationUrl + "\r\n\r\n",
	)))
	if err != nil {
		t.Fatalf("read request: %v", err)
	}

	record := &slog.Record{Message: "An HTTP response was served."}
	ctx := altshiftHttpContext.WithHttpContextValue(
		context.Background(),
		&altshiftHttpTypes.HttpContext{Request: request},
	)

	if err := tokenExtractor().Handle(ctx, record); err != nil {
		t.Fatalf("handle: %v", err)
	}

	text := recordText(t, record)
	if strings.Contains(text, maskingTestToken) {
		t.Errorf("the referer carried the token into the entry: %s", text)
	}
	if !strings.Contains(text, "/verification") {
		t.Errorf("the referring page was lost from the entry: %s", text)
	}
}

func TestExtractor_Handle_MasksReporting(t *testing.T) {
	t.Parallel()

	request, err := http.ReadRequest(bufio.NewReader(strings.NewReader(
		"POST /api/report/csp-report-to HTTP/1.1\r\nHost: dd.example.com\r\n\r\n",
	)))
	if err != nil {
		t.Fatalf("read request: %v", err)
	}

	record := &slog.Record{Message: "CSP violations were reported."}
	ctx := altshiftHttpContext.WithHttpContextValue(
		context.Background(),
		&altshiftHttpTypes.HttpContext{
			Request: request,
			Reporting: &altshiftSchemaTypes.HttpReporting{
				CspViolations: []*reporting_api.Report[*csp.CSPViolationReportBody]{
					{URL: verificationUrl, Body: &csp.CSPViolationReportBody{DocumentURL: verificationUrl}},
				},
			},
		},
	)

	if err := tokenExtractor().Handle(ctx, record); err != nil {
		t.Fatalf("handle: %v", err)
	}

	text := recordText(t, record)
	if strings.Contains(text, maskingTestToken) {
		t.Errorf("the reported document url carried the token into the entry: %s", text)
	}
	if !strings.Contains(text, "/verification") {
		t.Errorf("the reported page was lost from the entry: %s", text)
	}
}

// The message a violation is logged under names the URL that was blocked, and is written by the
// caller rather than built from the request.
func TestExtractor_Handle_MasksMessageUrls(t *testing.T) {
	t.Parallel()

	request, err := http.ReadRequest(bufio.NewReader(strings.NewReader(
		"POST /api/report/csp-report-to HTTP/1.1\r\nHost: dd.example.com\r\n\r\n",
	)))
	if err != nil {
		t.Fatalf("read request: %v", err)
	}

	record := &slog.Record{
		Message: `The page's settings blocked a script at ` + verificationUrl + ` from being executed`,
	}
	ctx := altshiftHttpContext.WithHttpContextValue(
		context.Background(),
		&altshiftHttpTypes.HttpContext{Request: request},
	)

	if err := tokenExtractor().Handle(ctx, record); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if strings.Contains(record.Message, maskingTestToken) {
		t.Errorf("the message kept the token: %s", record.Message)
	}
	if !strings.Contains(record.Message, "/verification") {
		t.Errorf("the message lost the page it was about: %s", record.Message)
	}
}
