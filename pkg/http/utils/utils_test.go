package utils

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	motmedelHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
	motmedelHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config/retry_config"
)

// serve starts an httptest server that is closed when the test ends.
func serve(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

// writeJSON encodes v as the response body. Handlers run in a separate goroutine,
// so it reports failures with t.Errorf (t.Fatal is unsafe off the test goroutine).
func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	if err := json.MarshalWrite(w, v); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func TestFetch_Success(t *testing.T) {
	t.Parallel()

	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if _, err := io.WriteString(w, "hello"); err != nil {
			t.Errorf("write: %v", err)
		}
	})

	resp, body, err := Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %v, want 200", resp)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q, want %q", body, "hello")
	}
}

func TestFetch_PostSendsMethodBodyAndHeaders(t *testing.T) {
	t.Parallel()

	// Assert on the received request inside the handler to avoid cross-goroutine
	// data sharing.
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if got := r.Header.Get("X-Test"); got != "value" {
			t.Errorf("X-Test = %q, want %q", got, "value")
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		if string(b) != "request-body" {
			t.Errorf("body = %q, want %q", b, "request-body")
		}
	})

	_, _, err := Fetch(
		context.Background(),
		server.URL,
		fetch_config.WithMethod(http.MethodPost),
		fetch_config.WithBody([]byte("request-body")),
		fetch_config.WithHeaders(map[string]string{"X-Test": "value"}),
	)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
}

func TestFetch_Non2xxStatusReturnsError(t *testing.T) {
	t.Parallel()

	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	resp, _, err := Fetch(context.Background(), server.URL)

	var non2xx *motmedelHttpErrors.Non2xxStatusCodeError
	if !errors.As(err, &non2xx) {
		t.Fatalf("err = %v, want *Non2xxStatusCodeError", err)
	}
	if non2xx.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", non2xx.StatusCode, http.StatusNotFound)
	}
	// The response is still returned alongside the error.
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Errorf("response = %v, want status 404", resp)
	}
}

func TestFetch_SkipErrorOnStatus(t *testing.T) {
	t.Parallel()

	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	resp, _, err := Fetch(context.Background(), server.URL, fetch_config.WithSkipErrorOnStatus(true))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %v, want 404", resp)
	}
}

func TestFetch_SkipReadResponseBody(t *testing.T) {
	t.Parallel()

	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.WriteString(w, "hello"); err != nil {
			t.Errorf("write: %v", err)
		}
	})

	resp, body, err := Fetch(context.Background(), server.URL, fetch_config.WithSkipReadResponseBody(true))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("body = %q, want empty (read skipped)", body)
	}
	// The body is left open for the caller when reading is skipped.
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

func TestFetch_EmptyURL(t *testing.T) {
	t.Parallel()

	if _, _, err := Fetch(context.Background(), ""); err == nil {
		t.Fatal("expected an error for an empty URL")
	}
}

func TestFetch_CanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := Fetch(ctx, "http://example.com")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// TestFetch_ContextTimeoutAbortsRequest verifies the request carries the caller's
// context (via NewRequestWithContext): a slow server is abandoned when the context
// deadline passes, rather than blocking until the server responds.
func TestFetch_ContextTimeoutAbortsRequest(t *testing.T) {
	t.Parallel()

	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		// Respond only much later, or as soon as the client goes away.
		select {
		case <-time.After(5 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, _, err := Fetch(ctx, server.URL)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestFetchWithRequest_Success(t *testing.T) {
	t.Parallel()

	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.WriteString(w, "ok"); err != nil {
			t.Errorf("write: %v", err)
		}
	})

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, body, err := FetchWithRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("FetchWithRequest: %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %v, want 200", resp)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want %q", body, "ok")
	}
}

type payload struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestFetchJson_Success(t *testing.T) {
	t.Parallel()

	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, payload{Name: "widget", Count: 5})
	})

	_, got, err := FetchJson[payload](context.Background(), server.URL)
	if err != nil {
		t.Fatalf("FetchJson: %v", err)
	}
	if got.Name != "widget" || got.Count != 5 {
		t.Fatalf("got = %+v, want {widget 5}", got)
	}
}

func TestFetchJson_SetsAcceptHeader(t *testing.T) {
	t.Parallel()

	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		writeJSON(t, w, payload{})
	})

	if _, _, err := FetchJson[payload](context.Background(), server.URL); err != nil {
		t.Fatalf("FetchJson: %v", err)
	}
}

func TestFetchJson_EmptyBodyReturnsZero(t *testing.T) {
	t.Parallel()

	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	_, got, err := FetchJson[payload](context.Background(), server.URL)
	if err != nil {
		t.Fatalf("FetchJson: %v", err)
	}
	if got != (payload{}) {
		t.Fatalf("got = %+v, want zero value", got)
	}
}

func TestFetchJson_InvalidJson(t *testing.T) {
	t.Parallel()

	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.WriteString(w, "{not valid json"); err != nil {
			t.Errorf("write: %v", err)
		}
	})

	if _, _, err := FetchJson[payload](context.Background(), server.URL); err == nil {
		t.Fatal("expected a JSON unmarshal error")
	}
}

func TestFetchJsonWithBody_MarshalsBodyAndSetsContentType(t *testing.T) {
	t.Parallel()

	type request struct {
		Query string `json:"query"`
	}
	type response struct {
		OK bool `json:"ok"`
	}

	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		var got request
		if err := json.UnmarshalRead(r.Body, &got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if got.Query != "hello" {
			t.Errorf("query = %q, want hello", got.Query)
		}
		writeJSON(t, w, response{OK: true})
	})

	_, got, err := FetchJsonWithBody[response](context.Background(), server.URL, request{Query: "hello"})
	if err != nil {
		t.Fatalf("FetchJsonWithBody: %v", err)
	}
	if !got.OK {
		t.Fatalf("got = %+v, want {true}", got)
	}
}

func TestFetch_RetriesThenSucceeds(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if _, err := io.WriteString(w, "ok"); err != nil {
			t.Errorf("write: %v", err)
		}
	})

	retryConfig := retry_config.New(retry_config.WithBaseDelay(time.Millisecond))
	resp, body, err := Fetch(context.Background(), server.URL, fetch_config.WithRetryConfig(retryConfig))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %v, want 200", resp)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want %q", body, "ok")
	}
	if got := attempts.Load(); got != 3 {
		t.Fatalf("attempts = %d, want 3", got)
	}
}

func TestFetch_RetryExhaustedReturnsError(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	})

	retryConfig := retry_config.New(retry_config.WithBaseDelay(time.Millisecond))
	_, _, err := Fetch(context.Background(), server.URL, fetch_config.WithRetryConfig(retryConfig))
	if err == nil {
		t.Fatal("expected an error after retries are exhausted")
	}
	// Default retry Count is 2, so 1 + 2 = 3 total attempts.
	if got := attempts.Load(); got != 3 {
		t.Fatalf("attempts = %d, want 3", got)
	}
	var non2xx *motmedelHttpErrors.Non2xxStatusCodeError
	if !errors.As(err, &non2xx) {
		t.Errorf("err = %v, want it to wrap *Non2xxStatusCodeError", err)
	}
}

func TestMakeStrongEtag(t *testing.T) {
	t.Parallel()

	// Quoted hex SHA-256 of "hello".
	want := `"2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"`
	got := MakeStrongEtag([]byte("hello"))
	if got != want {
		t.Fatalf("MakeStrongEtag = %s, want %s", got, want)
	}
	if second := MakeStrongEtag([]byte("hello")); second != got {
		t.Fatalf("not deterministic: %s != %s", second, got)
	}
	if other := MakeStrongEtag([]byte("world")); other == got {
		t.Fatal("different input produced the same etag")
	}
}

func TestBasicAuth(t *testing.T) {
	t.Parallel()

	// base64("user:pass").
	if got := BasicAuth("user", "pass"); got != "dXNlcjpwYXNz" {
		t.Fatalf("BasicAuth = %q, want %q", got, "dXNlcjpwYXNz")
	}
}

func TestGetSingleHeader(t *testing.T) {
	t.Parallel()

	header := http.Header{}
	header.Add("X-Single", "one")
	header.Add("X-Multi", "a")
	header.Add("X-Multi", "b")

	t.Run("single value (canonicalized lookup)", func(t *testing.T) {
		t.Parallel()
		got, err := GetSingleHeader("x-single", header)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "one" {
			t.Fatalf("got = %q, want %q", got, "one")
		}
	})

	t.Run("multiple values", func(t *testing.T) {
		t.Parallel()
		if _, err := GetSingleHeader("X-Multi", header); !errors.Is(err, motmedelHttpErrors.ErrMultipleHeaderValues) {
			t.Fatalf("err = %v, want ErrMultipleHeaderValues", err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		if _, err := GetSingleHeader("X-Absent", header); !errors.Is(err, motmedelHttpErrors.ErrMissingHeader) {
			t.Fatalf("err = %v, want ErrMissingHeader", err)
		}
	})

	t.Run("nil header", func(t *testing.T) {
		t.Parallel()
		if _, err := GetSingleHeader("X-Single", nil); err == nil {
			t.Fatal("expected an error for a nil header")
		}
	})
}

func TestRetryAfterHeaderDelay(t *testing.T) {
	t.Parallel()

	t.Run("nil header", func(t *testing.T) {
		t.Parallel()
		if d := retryAfterHeaderDelay(nil); d != nil {
			t.Fatalf("got %v, want nil", d)
		}
	})

	t.Run("absent", func(t *testing.T) {
		t.Parallel()
		if d := retryAfterHeaderDelay(http.Header{}); d != nil {
			t.Fatalf("got %v, want nil", d)
		}
	})

	t.Run("delay-seconds (rounded up by one)", func(t *testing.T) {
		t.Parallel()
		h := http.Header{}
		h.Set("Retry-After", "5")
		if d := retryAfterHeaderDelay(h); d == nil || *d != 6*time.Second {
			t.Fatalf("got %v, want 6s", d)
		}
	})

	t.Run("http-date relative to Date header", func(t *testing.T) {
		t.Parallel()
		h := http.Header{}
		h.Set("Date", "Mon, 02 Jan 2006 15:04:05 GMT")
		h.Set("Retry-After", "Mon, 02 Jan 2006 15:04:15 GMT")
		if d := retryAfterHeaderDelay(h); d == nil || *d != 10*time.Second {
			t.Fatalf("got %v, want 10s", d)
		}
	})

	t.Run("invalid value", func(t *testing.T) {
		t.Parallel()
		h := http.Header{}
		h.Set("Retry-After", "not-a-thing")
		if d := retryAfterHeaderDelay(h); d != nil {
			t.Fatalf("got %v, want nil", d)
		}
	})
}

func TestRetryWaitDuration(t *testing.T) {
	t.Parallel()

	retryAfterHeader := func(value string) http.Header {
		return http.Header{"Retry-After": {value}}
	}

	t.Run("exponential backoff", func(t *testing.T) {
		t.Parallel()
		config := &retry_config.Config{BaseDelay: 100 * time.Millisecond}
		for _, tc := range []struct {
			i    int
			want time.Duration
		}{
			{1, 100 * time.Millisecond},
			{2, 200 * time.Millisecond},
			{3, 400 * time.Millisecond},
		} {
			if delay, giveUp := retryWaitDuration(config, nil, nil, tc.i); giveUp || delay != tc.want {
				t.Errorf("i=%d: (%v, %v), want (%v, false)", tc.i, delay, giveUp, tc.want)
			}
		}
	})

	t.Run("backoff clamped to maximum", func(t *testing.T) {
		t.Parallel()
		config := &retry_config.Config{BaseDelay: time.Second, MaximumWaitTime: 5 * time.Second}
		// 1s * 2^4 = 16s, clamped to 5s.
		if delay, giveUp := retryWaitDuration(config, nil, nil, 5); giveUp || delay != 5*time.Second {
			t.Fatalf("(%v, %v), want (5s, false)", delay, giveUp)
		}
	})

	t.Run("retry-after header takes precedence over backoff", func(t *testing.T) {
		t.Parallel()
		config := &retry_config.Config{BaseDelay: time.Second}
		// "1" -> 1s + 1s rounding = 2s.
		if delay, giveUp := retryWaitDuration(config, &http.Response{Header: retryAfterHeader("1")}, nil, 1); giveUp || delay != 2*time.Second {
			t.Fatalf("(%v, %v), want (2s, false)", delay, giveUp)
		}
	})

	t.Run("give up when advised delay exceeds maximum", func(t *testing.T) {
		t.Parallel()
		config := &retry_config.Config{BaseDelay: time.Second, MaximumWaitTime: 5 * time.Second}
		// "10" -> 11s advised > 5s maximum.
		if _, giveUp := retryWaitDuration(config, &http.Response{Header: retryAfterHeader("10")}, nil, 1); !giveUp {
			t.Fatal("expected giveUp = true")
		}
	})

	t.Run("RetryAfterFunc takes precedence over header", func(t *testing.T) {
		t.Parallel()
		advised := 3 * time.Second
		config := &retry_config.Config{
			BaseDelay:      time.Second,
			RetryAfterFunc: func(*http.Response, []byte) *time.Duration { return &advised },
		}
		if delay, giveUp := retryWaitDuration(config, &http.Response{Header: retryAfterHeader("100")}, nil, 1); giveUp || delay != advised {
			t.Fatalf("(%v, %v), want (3s, false)", delay, giveUp)
		}
	})
}

func TestFetch_HTTPS(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.WriteString(w, "secure"); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	// server.Client() trusts the server's self-signed certificate.
	resp, body, err := Fetch(context.Background(), server.URL, fetch_config.WithHttpClient(server.Client()))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %v, want 200", resp)
	}
	if string(body) != "secure" {
		t.Fatalf("body = %q, want %q", body, "secure")
	}
	if resp.TLS == nil {
		t.Fatal("expected a TLS connection state on the response")
	}
}

func TestGetMatchingContentEncoding(t *testing.T) {
	t.Parallel()

	enc := func(coding string, quality float32) *motmedelHttpTypes.Encoding {
		return &motmedelHttpTypes.Encoding{Coding: coding, QualityValue: quality}
	}

	testCases := []struct {
		name   string
		client []*motmedelHttpTypes.Encoding
		server []string
		want   string
	}{
		{
			name:   "no client encodings yields identity",
			client: nil,
			server: []string{"gzip"},
			want:   AcceptContentIdentity,
		},
		{
			name:   "exact match",
			client: []*motmedelHttpTypes.Encoding{enc("gzip", 1)},
			server: []string{"gzip", "br"},
			want:   "gzip",
		},
		{
			name:   "wildcard picks first server encoding",
			client: []*motmedelHttpTypes.Encoding{enc("*", 1)},
			server: []string{"br", "gzip"},
			want:   "br",
		},
		{
			name:   "identity explicitly accepted",
			client: []*motmedelHttpTypes.Encoding{enc("identity", 1)},
			server: []string{"gzip"},
			want:   AcceptContentIdentity,
		},
		{
			name:   "no match falls back to identity",
			client: []*motmedelHttpTypes.Encoding{enc("gzip", 1)},
			server: []string{"br"},
			want:   AcceptContentIdentity,
		},
		{
			name:   "identity disallowed with no match yields empty",
			client: []*motmedelHttpTypes.Encoding{enc("*", 0)},
			server: []string{"gzip"},
			want:   "",
		},
		{
			// Browsers list gzip first with equal quality; the server's
			// preference (smallest variant first) decides within the group.
			name:   "equal quality defers to server preference",
			client: []*motmedelHttpTypes.Encoding{enc("gzip", 1), enc("deflate", 1), enc("br", 1), enc("zstd", 1)},
			server: []string{"br", "gzip"},
			want:   "br",
		},
		{
			name:   "higher client quality beats server preference",
			client: []*motmedelHttpTypes.Encoding{enc("gzip", 1), enc("br", 0.5)},
			server: []string{"br", "gzip"},
			want:   "gzip",
		},
		{
			name:   "zero quality excludes an encoding",
			client: []*motmedelHttpTypes.Encoding{enc("gzip", 1), enc("br", 0)},
			server: []string{"br", "gzip"},
			want:   "gzip",
		},
		{
			name:   "wildcard does not resurrect an excluded encoding",
			client: []*motmedelHttpTypes.Encoding{enc("br", 0), enc("*", 1)},
			server: []string{"br", "gzip"},
			want:   "gzip",
		},
		{
			name:   "equal quality identity loses to a supported encoding",
			client: []*motmedelHttpTypes.Encoding{enc("identity", 1), enc("gzip", 1)},
			server: []string{"gzip"},
			want:   "gzip",
		},
		{
			name:   "higher quality identity wins",
			client: []*motmedelHttpTypes.Encoding{enc("identity", 1), enc("gzip", 0.5)},
			server: []string{"gzip"},
			want:   AcceptContentIdentity,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := GetMatchingContentEncoding(tc.client, tc.server); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGetMatchingAccept(t *testing.T) {
	t.Parallel()

	client := func(typ, subtype string) *motmedelHttpTypes.MediaRange {
		return &motmedelHttpTypes.MediaRange{Type: typ, Subtype: subtype}
	}
	server := func(typ, subtype string) *motmedelHttpTypes.ServerMediaRange {
		return &motmedelHttpTypes.ServerMediaRange{Type: typ, Subtype: subtype}
	}

	testCases := []struct {
		name        string
		client      []*motmedelHttpTypes.MediaRange
		server      []*motmedelHttpTypes.ServerMediaRange
		wantType    string
		wantSubtype string
		wantNil     bool
	}{
		{
			name:    "no client ranges",
			client:  nil,
			server:  []*motmedelHttpTypes.ServerMediaRange{server("application", "json")},
			wantNil: true,
		},
		{
			name:    "no server ranges",
			client:  []*motmedelHttpTypes.MediaRange{client("application", "json")},
			server:  nil,
			wantNil: true,
		},
		{
			name:        "full wildcard picks first server range",
			client:      []*motmedelHttpTypes.MediaRange{client("*", "*")},
			server:      []*motmedelHttpTypes.ServerMediaRange{server("application", "json"), server("text", "html")},
			wantType:    "application",
			wantSubtype: "json",
		},
		{
			name:        "exact match",
			client:      []*motmedelHttpTypes.MediaRange{client("application", "json")},
			server:      []*motmedelHttpTypes.ServerMediaRange{server("text", "html"), server("application", "json")},
			wantType:    "application",
			wantSubtype: "json",
		},
		{
			name:        "subtype wildcard",
			client:      []*motmedelHttpTypes.MediaRange{client("application", "*")},
			server:      []*motmedelHttpTypes.ServerMediaRange{server("application", "xml")},
			wantType:    "application",
			wantSubtype: "xml",
		},
		{
			name:    "no match",
			client:  []*motmedelHttpTypes.MediaRange{client("text", "plain")},
			server:  []*motmedelHttpTypes.ServerMediaRange{server("application", "json")},
			wantNil: true,
		},
		{
			name:        "nil client entry is skipped",
			client:      []*motmedelHttpTypes.MediaRange{nil, client("application", "json")},
			server:      []*motmedelHttpTypes.ServerMediaRange{server("application", "json")},
			wantType:    "application",
			wantSubtype: "json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := GetMatchingAccept(tc.client, tc.server)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("got %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("got nil, want %s/%s", tc.wantType, tc.wantSubtype)
			}
			if got.Type != tc.wantType || got.Subtype != tc.wantSubtype {
				t.Fatalf("got %s/%s, want %s/%s", got.Type, got.Subtype, tc.wantType, tc.wantSubtype)
			}
		})
	}
}

func TestParseLastModifiedTimestamp(t *testing.T) {
	t.Parallel()

	t.Run("valid RFC1123", func(t *testing.T) {
		t.Parallel()
		got, err := ParseLastModifiedTimestamp("Mon, 02 Jan 2006 15:04:05 GMT")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := time.Date(2006, time.January, 2, 15, 4, 5, 0, time.UTC); !got.Equal(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()
		if _, err := ParseLastModifiedTimestamp("not a date"); !errors.Is(err, motmedelHttpErrors.ErrBadIfModifiedSinceTimestamp) {
			t.Fatalf("err = %v, want ErrBadIfModifiedSinceTimestamp", err)
		}
	})
}

func TestIfNoneMatchCacheHit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		ifNoneMatch string
		etag        string
		want        bool
	}{
		{name: "empty if-none-match", ifNoneMatch: "", etag: `"abc"`, want: false},
		{name: "empty etag", ifNoneMatch: `"abc"`, etag: "", want: false},
		{name: "match", ifNoneMatch: `"abc"`, etag: `"abc"`, want: true},
		{name: "mismatch", ifNoneMatch: `"abc"`, etag: `"def"`, want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IfNoneMatchCacheHit(tc.ifNoneMatch, tc.etag); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIfModifiedSinceCacheHit(t *testing.T) {
	t.Parallel()

	const (
		earlier = "Mon, 02 Jan 2006 15:04:05 GMT"
		later   = "Mon, 02 Jan 2006 15:04:15 GMT"
	)

	t.Run("empty values are a miss", func(t *testing.T) {
		t.Parallel()
		if hit, err := IfModifiedSinceCacheHit("", later); err != nil || hit {
			t.Fatalf("(%v, %v), want (false, nil)", hit, err)
		}
	})

	t.Run("not modified since (hit)", func(t *testing.T) {
		t.Parallel()
		// Last-Modified before If-Modified-Since => resource unchanged.
		if hit, err := IfModifiedSinceCacheHit(later, earlier); err != nil || !hit {
			t.Fatalf("(%v, %v), want (true, nil)", hit, err)
		}
	})

	t.Run("equal timestamps (hit)", func(t *testing.T) {
		t.Parallel()
		if hit, err := IfModifiedSinceCacheHit(earlier, earlier); err != nil || !hit {
			t.Fatalf("(%v, %v), want (true, nil)", hit, err)
		}
	})

	t.Run("modified since (miss)", func(t *testing.T) {
		t.Parallel()
		// Last-Modified after If-Modified-Since => resource changed.
		if hit, err := IfModifiedSinceCacheHit(earlier, later); err != nil || hit {
			t.Fatalf("(%v, %v), want (false, nil)", hit, err)
		}
	})

	t.Run("invalid If-Modified-Since", func(t *testing.T) {
		t.Parallel()
		if _, err := IfModifiedSinceCacheHit("garbage", later); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("invalid Last-Modified", func(t *testing.T) {
		t.Parallel()
		if _, err := IfModifiedSinceCacheHit(later, "garbage"); err == nil {
			t.Fatal("expected an error")
		}
	})
}
