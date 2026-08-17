package log

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json/v2"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/url"
	"reflect"
	"testing"
	"time"

	context2 "github.com/altshiftab/utils_go/pkg/context"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftLogHandler "github.com/altshiftab/utils_go/pkg/log/handler"
)

var (
	errBoom      = errors.New("boom")
	errPlain     = errors.New("plain")
	errBase      = errors.New("base")
	errRefused   = errors.New("refused")
	errDenied    = errors.New("denied")
	errUntrusted = errors.New("untrusted")
)

func dropTime(groups []string, attr slog.Attr) slog.Attr {
	if len(groups) == 0 && attr.Key == slog.TimeKey {
		return slog.Attr{}
	}
	return attr
}

// renderAttrs logs a record carrying attrs through a plain JSON handler and
// returns the parsed payload with the builtin level/msg keys removed.
func renderAttrs(t *testing.T, attrs []any) map[string]any {
	t.Helper()
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{ReplaceAttr: dropTime}))
	logger.Info("m", attrs...)

	m := parseJSON(t, buf)
	delete(m, slog.LevelKey)
	delete(m, slog.MessageKey)
	return m
}

func parseJSON(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
		t.Fatalf("failed to parse %q: %v", buf.String(), err)
	}
	return m
}

// anyAttrsToMap resolves a []any of slog.Attr (native go values) into a map.
func anyAttrsToMap(t *testing.T, attrs []any) map[string]any {
	t.Helper()
	m := make(map[string]any, len(attrs))
	for _, raw := range attrs {
		attr, ok := raw.(slog.Attr)
		if !ok {
			t.Fatalf("expected slog.Attr, got %T", raw)
		}
		value := attr.Value.Resolve()
		if value.Kind() == slog.KindGroup {
			sub := make([]any, 0, len(value.Group()))
			for _, groupAttr := range value.Group() {
				sub = append(sub, groupAttr)
			}
			m[attr.Key] = anyAttrsToMap(t, sub)
		} else {
			m[attr.Key] = value.Any()
		}
	}
	return m
}

// dig walks nested map[string]any values, failing the test on a missing key.
func dig(t *testing.T, m map[string]any, keys ...string) any {
	t.Helper()
	var current any = m
	for _, key := range keys {
		asMap, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("expected map at key %q, got %T (%#v)", key, current, current)
		}
		current, ok = asMap[key]
		if !ok {
			t.Fatalf("missing key %q in %#v", key, asMap)
		}
	}
	return current
}

func TestAttrsFromMap(t *testing.T) {
	t.Parallel()

	attrs := AttrsFromMap(map[string]any{
		"a":      "1",
		"nested": map[string]any{"b": "2", "deep": map[string]any{"c": "3"}},
	})

	got := renderAttrs(t, attrs)
	want := map[string]any{
		"a": "1",
		"nested": map[string]any{
			"b":    "2",
			"deep": map[string]any{"c": "3"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestContextExtractorFunction(t *testing.T) {
	t.Parallel()

	called := false
	var fn ContextExtractor = ContextExtractorFunction(func(_ context.Context, record *slog.Record) error {
		called = true
		record.Add("injected", "yes")
		return nil
	})

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "m", 0)
	if err := fn.Handle(context.Background(), &record); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("extractor function was not called")
	}
	if got := recordAttrs(record); got["injected"] != "yes" {
		t.Fatalf("expected injected=yes, got %#v", got)
	}
}

func recordAttrs(record slog.Record) map[string]any {
	m := make(map[string]any)
	record.Attrs(func(attr slog.Attr) bool {
		m[attr.Key] = attr.Value.Any()
		return true
	})
	return m
}

type recordingHandler struct {
	called bool
	record slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, record slog.Record) error {
	h.called = true
	h.record = record.Clone()
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func TestContextHandlerHandleRunsExtractors(t *testing.T) {
	t.Parallel()

	next := &recordingHandler{}
	ch := &ContextHandler{
		Next: next,
		Extractors: []ContextExtractor{
			nil, // must be skipped without panicking
			ContextExtractorFunction(func(_ context.Context, record *slog.Record) error {
				record.Add("injected", "yes")
				return nil
			}),
		},
	}

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "m", 0)
	if err := ch.Handle(context.Background(), record); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !next.called {
		t.Fatal("next handler was not called")
	}
	if got := recordAttrs(next.record); got["injected"] != "yes" {
		t.Fatalf("expected injected=yes forwarded to next, got %#v", got)
	}
}

func TestContextHandlerHandleExtractorError(t *testing.T) {
	t.Parallel()

	next := &recordingHandler{}
	ch := &ContextHandler{
		Next: next,
		Extractors: []ContextExtractor{
			ContextExtractorFunction(func(context.Context, *slog.Record) error {
				return errBoom
			}),
		},
	}

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "m", 0)
	err := ch.Handle(context.Background(), record)
	if err == nil {
		t.Fatal("expected error from failing extractor")
	}
	if next.called {
		t.Fatal("next handler must not be called when an extractor fails")
	}
	if !errorContains(err, "extractor handle") {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}

func errorContains(err error, substr string) bool {
	return err != nil && bytes.Contains([]byte(err.Error()), []byte(substr))
}

func TestContextHandlerEnabledDelegates(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	ch := &ContextHandler{Next: slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelWarn})}
	if ch.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("expected Info disabled")
	}
	if !ch.Enabled(context.Background(), slog.LevelWarn) {
		t.Fatal("expected Warn enabled")
	}
}

func TestContextHandlerWithGroupAndAttrs(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	ch := &ContextHandler{Next: slog.NewJSONHandler(buf, &slog.HandlerOptions{ReplaceAttr: dropTime})}
	logger := slog.New(ch).WithGroup("g").With("base", "v")
	logger.Info("m", "a", "b")

	m := parseJSON(t, buf)
	if got := dig(t, m, "g", "base"); got != "v" {
		t.Fatalf("g.base = %v, want v", got)
	}
	if got := dig(t, m, "g", "a"); got != "b" {
		t.Fatalf("g.a = %v, want b", got)
	}
}

func TestMakeErrorAttrs(t *testing.T) {
	t.Parallel()

	customErr := &customError{msg: "custom failure"}

	testCases := []struct {
		name      string
		extractor *ErrorContextExtractor
		err       error
		assert    func(t *testing.T, m map[string]any)
	}{
		{
			name:      "nil error yields nothing",
			extractor: &ErrorContextExtractor{},
			err:       nil,
			assert: func(t *testing.T, m map[string]any) {
				if len(m) != 0 {
					t.Fatalf("expected empty attrs, got %#v", m)
				}
			},
		},
		{
			name:      "plain error has message and no type",
			extractor: &ErrorContextExtractor{},
			err:       errPlain,
			assert: func(t *testing.T, m map[string]any) {
				if m["message"] != "plain" {
					t.Fatalf("message = %v, want plain", m["message"])
				}
				if _, ok := m["type"]; ok {
					t.Fatalf("did not expect type for plain error: %#v", m)
				}
			},
		},
		{
			name:      "custom typed error has type",
			extractor: &ErrorContextExtractor{},
			err:       customErr,
			assert: func(t *testing.T, m map[string]any) {
				want := reflect.TypeFor[*customError]().String()
				if m["type"] != want {
					t.Fatalf("type = %v, want %v", m["type"], want)
				}
				if m["message"] != "custom failure" {
					t.Fatalf("message = %v", m["message"])
				}
			},
		},
		{
			name:      "code id and stack trace",
			extractor: &ErrorContextExtractor{},
			err:       &altshiftErrors.Error{Message: "m", Code: "C1", Id: "I1", StackTrace: "trace"},
			assert: func(t *testing.T, m map[string]any) {
				if m["code"] != "C1" || m["id"] != "I1" || m["stack_trace"] != "trace" {
					t.Fatalf("unexpected attrs: %#v", m)
				}
			},
		},
		{
			name:      "skip stack trace",
			extractor: &ErrorContextExtractor{SkipStackTrace: true},
			err:       &altshiftErrors.Error{Message: "m", StackTrace: "trace"},
			assert: func(t *testing.T, m map[string]any) {
				if _, ok := m["stack_trace"]; ok {
					t.Fatalf("stack_trace should be skipped: %#v", m)
				}
			},
		},
		{
			name:      "input included",
			extractor: &ErrorContextExtractor{},
			err:       &altshiftErrors.Error{Message: "m", Input: "hello"},
			assert: func(t *testing.T, m map[string]any) {
				if got := dig(t, m, "input", "value"); got != "hello" {
					t.Fatalf("input.value = %v, want hello", got)
				}
				if got := dig(t, m, "input", "type"); got != "string" {
					t.Fatalf("input.type = %v, want string", got)
				}
			},
		},
		{
			name:      "skip input",
			extractor: &ErrorContextExtractor{SkipInput: true},
			err:       &altshiftErrors.Error{Message: "m", Input: "hello"},
			assert: func(t *testing.T, m map[string]any) {
				if _, ok := m["input"]; ok {
					t.Fatalf("input should be skipped: %#v", m)
				}
			},
		},
		{
			name:      "cause chain",
			extractor: &ErrorContextExtractor{},
			err:       &altshiftErrors.Error{Message: "mid", Cause: errBase},
			assert: func(t *testing.T, m map[string]any) {
				if m["message"] != "mid" {
					t.Fatalf("message = %v, want mid", m["message"])
				}
				if got := dig(t, m, "cause", "message"); got != "base" {
					t.Fatalf("cause.message = %v, want base", got)
				}
			},
		},
		{
			name:      "skip cause",
			extractor: &ErrorContextExtractor{SkipCause: true},
			err:       &altshiftErrors.Error{Message: "mid", Cause: errBase},
			assert: func(t *testing.T, m map[string]any) {
				if _, ok := m["cause"]; ok {
					t.Fatalf("cause should be skipped: %#v", m)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			attrs := testCase.extractor.MakeErrorAttrs(testCase.err)
			testCase.assert(t, renderAttrs(t, attrs))
		})
	}
}

type customError struct {
	msg string
}

func (e *customError) Error() string { return e.msg }

func TestMakeNetAddrAttrs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		addr net.Addr
		want map[string]any
	}{
		{
			name: "tcp",
			addr: &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 111},
			want: map[string]any{"ip": "1.2.3.4", "port": int64(111)},
		},
		{
			name: "udp",
			addr: &net.UDPAddr{IP: net.ParseIP("5.6.7.8"), Port: 222},
			want: map[string]any{"ip": "5.6.7.8", "port": int64(222)},
		},
		{
			name: "other",
			addr: &net.IPAddr{IP: net.ParseIP("9.9.9.9")},
			want: map[string]any{"address": "9.9.9.9"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got := anyAttrsToMap(t, makeNetAddrAttrs(testCase.addr))
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("got %#v, want %#v", got, testCase.want)
			}
		})
	}
}

func TestMakeNetworkAttrs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  *net.OpError
		want map[string]any
	}{
		{
			name: "tcp4",
			err:  &net.OpError{Net: "tcp4"},
			want: map[string]any{"transport": "tcp", "iana_number": "6", "type": "ipv4"},
		},
		{
			name: "udp6",
			err:  &net.OpError{Net: "udp6"},
			want: map[string]any{"transport": "udp", "iana_number": "17", "type": "ipv6"},
		},
		{
			name: "tcp with ipv6 addr",
			err:  &net.OpError{Net: "tcp", Addr: &net.TCPAddr{IP: net.ParseIP("::1")}},
			want: map[string]any{"transport": "tcp", "iana_number": "6", "type": "ipv6"},
		},
		{
			name: "empty net",
			err:  &net.OpError{Net: ""},
			want: map[string]any{},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got := anyAttrsToMap(t, makeNetworkAttrs(testCase.err))
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("got %#v, want %#v", got, testCase.want)
			}
		})
	}
}

func TestMakeTlsCertAttrs(t *testing.T) {
	t.Parallel()

	if got := makeTlsCertAttrs(nil); got != nil {
		t.Fatalf("expected nil for nil cert, got %#v", got)
	}

	notBefore := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	notAfter := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	cert := &x509.Certificate{
		Subject:   pkix.Name{CommonName: "cn"},
		Issuer:    pkix.Name{CommonName: "iss"},
		NotBefore: notBefore,
		NotAfter:  notAfter,
	}

	got := anyAttrsToMap(t, makeTlsCertAttrs(cert))
	want := map[string]any{
		"subject":    "CN=cn",
		"issuer":     "CN=iss",
		"not_after":  notAfter.UTC().Format(time.RFC3339Nano),
		"not_before": notBefore.UTC().Format(time.RFC3339Nano),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

// logWithError runs err through the ErrorContextExtractor.Handle pipeline
// (context handler + tree handler + JSON) and returns the parsed output.
func logWithError(t *testing.T, extractor *ErrorContextExtractor, err error) map[string]any {
	t.Helper()
	buf := &bytes.Buffer{}
	next := altshiftLogHandler.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{ReplaceAttr: dropTime}))
	ch := &ContextHandler{Next: next, Extractors: []ContextExtractor{extractor}}
	logger := slog.New(ch)
	logger.ErrorContext(context2.WithError(context.Background(), err), "boom")
	return parseJSON(t, buf)
}

func TestHandleNilRecord(t *testing.T) {
	t.Parallel()
	extractor := &ErrorContextExtractor{}
	if err := extractor.Handle(context.Background(), nil); err != nil {
		t.Fatalf("expected nil error for nil record, got %v", err)
	}
}

func TestHandleNoErrorInContext(t *testing.T) {
	t.Parallel()
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "m", 0)
	extractor := &ErrorContextExtractor{}
	if err := extractor.Handle(context.Background(), &record); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.NumAttrs() != 0 {
		t.Fatalf("expected no attrs added, got %d", record.NumAttrs())
	}
}

func TestHandlePlainError(t *testing.T) {
	t.Parallel()
	m := logWithError(t, &ErrorContextExtractor{}, errPlain)
	if got := dig(t, m, "error", "message"); got != "plain" {
		t.Fatalf("error.message = %v, want plain", got)
	}
}

func TestHandleOpError(t *testing.T) {
	t.Parallel()

	opErr := &net.OpError{
		Op:     "read",
		Net:    "tcp",
		Source: &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 111},
		Addr:   &net.TCPAddr{IP: net.ParseIP("5.6.7.8"), Port: 222},
		Err:    errRefused,
	}
	m := logWithError(t, &ErrorContextExtractor{}, opErr)

	if got := dig(t, m, "error", "context", "client", "ip"); got != "1.2.3.4" {
		t.Fatalf("client.ip = %v", got)
	}
	if got := dig(t, m, "error", "context", "client", "port"); got != float64(111) {
		t.Fatalf("client.port = %v", got)
	}
	if got := dig(t, m, "error", "context", "server", "ip"); got != "5.6.7.8" {
		t.Fatalf("server.ip = %v", got)
	}
	if got := dig(t, m, "error", "context", "network", "transport"); got != "tcp" {
		t.Fatalf("network.transport = %v", got)
	}
	if got := dig(t, m, "error", "context", "network", "type"); got != "ipv4" {
		t.Fatalf("network.type = %v", got)
	}
}

func TestHandleDNSError(t *testing.T) {
	t.Parallel()

	dnsErr := &net.DNSError{Name: "example.com", Server: "8.8.8.8:53", Err: "no such host"}
	m := logWithError(t, &ErrorContextExtractor{}, dnsErr)

	if got := dig(t, m, "error", "context", "dns", "question", "name"); got != "example.com" {
		t.Fatalf("dns.question.name = %v", got)
	}
	if got := dig(t, m, "error", "context", "server", "address"); got != "8.8.8.8:53" {
		t.Fatalf("server.address = %v", got)
	}
}

func TestHandleURLError(t *testing.T) {
	t.Parallel()

	urlErr := &url.Error{Op: "Get", URL: "http://example.com", Err: errBoom}
	m := logWithError(t, &ErrorContextExtractor{}, urlErr)

	if got := dig(t, m, "error", "context", "url", "original"); got != "http://example.com" {
		t.Fatalf("url.original = %v", got)
	}
	if got := dig(t, m, "error", "context", "http", "request", "method"); got != "GET" {
		t.Fatalf("http.request.method = %v", got)
	}
}

func TestHandlePathError(t *testing.T) {
	t.Parallel()

	pathErr := &fs.PathError{Op: "open", Path: "/etc/secret", Err: errDenied}
	m := logWithError(t, &ErrorContextExtractor{}, pathErr)

	if got := dig(t, m, "error", "context", "file", "path"); got != "/etc/secret" {
		t.Fatalf("file.path = %v", got)
	}
}

func TestHandleCertificateVerificationError(t *testing.T) {
	t.Parallel()

	cert := &x509.Certificate{Subject: pkix.Name{CommonName: "cn"}, Issuer: pkix.Name{CommonName: "iss"}}
	certErr := &tls.CertificateVerificationError{
		UnverifiedCertificates: []*x509.Certificate{cert},
		Err:                    errUntrusted,
	}
	m := logWithError(t, &ErrorContextExtractor{}, certErr)

	if got := dig(t, m, "error", "context", "tls", "server", "subject"); got != "CN=cn" {
		t.Fatalf("tls.server.subject = %v", got)
	}
}

type ctxKeyType struct{}

var ctxKey ctxKeyType

func TestHandleContextError(t *testing.T) {
	t.Parallel()

	innerCtx := context.WithValue(context.Background(), ctxKey, "ctxval")
	extErr := altshiftErrors.NewCtx(innerCtx, "boom")

	sub := ContextExtractorFunction(func(ctx context.Context, record *slog.Record) error {
		if v, ok := ctx.Value(ctxKey).(string); ok {
			record.Add("extracted", v)
		}
		return nil
	})
	extractor := &ErrorContextExtractor{ContextExtractors: []ContextExtractor{sub}}

	m := logWithError(t, extractor, extErr)
	if got := dig(t, m, "error", "context", "extracted"); got != "ctxval" {
		t.Fatalf("error.context.extracted = %v, want ctxval", got)
	}
}
