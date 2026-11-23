package mux

import (
	"bytes"
	"context"
	"crypto/tls"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/http_context_extractor"
	altshiftLog "github.com/altshiftab/utils_go/pkg/log"
	"github.com/altshiftab/utils_go/pkg/log/context_logger"
)

// captureVhostLog serves the request through the vhost mux with the default
// logger writing JSON through the HTTP context extractor, as a service logging
// its requests does, and returns what was logged. The default logger is global,
// so nothing in this file runs in parallel.
func captureVhostLog(t *testing.T, vhostMux *VhostMux, request *http.Request) (string, int) {
	t.Helper()

	var buffer bytes.Buffer
	extractor := &http_context_extractor.Extractor{}
	logger := context_logger.New(
		slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelInfo}),
		&altshiftLog.ErrorContextExtractor{ContextExtractors: []altshiftLog.ContextExtractor{extractor}},
		extractor,
	)

	previous := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(previous) })

	recorder := httptest.NewRecorder()
	vhostMux.ServeHTTP(recorder, request)

	return buffer.String(), recorder.Code
}

// A request no vhost answers for comes from anyone, before anything has been
// asked of them, so what is logged of it is a summary of a bounded size rather
// than the request as it came: the size of its headers is the client's to
// choose.
//
//nolint:paralleltest // Swaps the default logger, which is global, to read what is logged.
func TestVhostMux_MisdirectedRequestLogIsBounded(t *testing.T) {
	const servedHost = "open.home.arpa"
	const otherHost = "restic.home.arpa"
	const remoteAddr = "192.0.2.10:54321"
	const logLimit = 8 * 1024

	padding := strings.Repeat("x", 1<<20)

	testCases := []struct {
		name       string
		serverName string
		host       string
		path       string
		query      string
		headers    map[string]string
		lines      map[string][]string
		wantLogged []string
		wantAbsent []string
	}{
		{
			name:       "a host other than the server name, with large headers",
			serverName: otherHost,
			host:       servedHost,
			path:       "/",
			headers:    map[string]string{"X-Padding": padding, "User-Agent": "probe/1.0 " + padding},
			wantLogged: []string{otherHost, servedHost, "192.0.2.10", "probe/1.0"},
		},
		{
			name:       "a host no vhost answers for, with large headers",
			serverName: "unknown.home.arpa",
			host:       "unknown.home.arpa",
			path:       "/",
			headers:    map[string]string{"X-Padding-1": padding, "X-Padding-2": padding},
			wantLogged: []string{"unknown.home.arpa", "192.0.2.10"},
		},
		{
			name:       "a large host",
			serverName: servedHost,
			host:       servedHost + padding,
			path:       "/",
			wantLogged: []string{servedHost, "192.0.2.10"},
		},
		{
			name:       "a large path and query",
			serverName: otherHost,
			host:       servedHost,
			path:       "/" + padding,
			query:      "q=" + padding + "&secret=hunter2",
			wantLogged: []string{otherHost, servedHost, "192.0.2.10"},
			wantAbsent: []string{"hunter2"},
		},
		{
			// The query was meant for a vhost that may declare what in it is
			// secret, so none of it is logged, however short.
			name:       "a short query",
			serverName: otherHost,
			host:       servedHost,
			path:       "/login",
			query:      "token=query-marker",
			wantLogged: []string{"/login"},
			wantAbsent: []string{"query-marker"},
		},
		{
			name:       "what a load balancer adds is kept, and a cookie is not",
			serverName: otherHost,
			host:       servedHost,
			path:       "/",
			headers: map[string]string{
				"X-Cloud-Trace-Context":         "105445aa7843bc8bf206b12000100000/1;o=1",
				"X-Client-Geo-Country-Iso-Code": "ZZ-GEO-MARKER",
				"X-Tls-Ja4-Fingerprint":         "t13d1516h2_ja4marker",
				"Cookie":                        "session=cookie-marker",
				"X-Padding":                     padding,
			},
			wantLogged: []string{"105445aa7843bc8bf206b12000100000", "ZZ-GEO-MARKER", "t13d1516h2_ja4marker"},
			wantAbsent: []string{"cookie-marker"},
		},
		{
			name:       "the forwarded client is kept, bounded",
			serverName: otherHost,
			host:       servedHost,
			path:       "/",
			headers:    map[string]string{"X-Forwarded-For": "198.51.100.7, " + padding},
			wantLogged: []string{"198.51.100.7"},
		},
		{
			name:       "the entry a proxy appended is kept after a long client value",
			serverName: otherHost,
			host:       servedHost,
			path:       "/",
			headers:    map[string]string{"X-Forwarded-For": strings.Repeat("x", 300) + ", 198.51.100.8"},
			wantLogged: []string{"198.51.100.8"},
		},
		{
			name:       "every line of a header is kept",
			serverName: otherHost,
			host:       servedHost,
			path:       "/",
			lines: map[string][]string{
				"X-Forwarded-For": {"", "198.51.100.9"},
				"Forwarded":       {"for=203.0.113.1", "for=203.0.113.99"},
			},
			wantLogged: []string{"198.51.100.9", "203.0.113.1", "203.0.113.99"},
		},
	}

	//nolint:paralleltest // Same reason: each case swaps the default logger for one it can read.
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			vhostMux := &VhostMux{
				HostToSpecification: map[string]*VhostMuxSpecification{
					servedHost: {Mux: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
						t.Error("the request reached a vhost")
					})},
				},
			}

			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+servedHost+"/", nil)
			request.Host = testCase.host
			request.URL.Path = testCase.path
			request.URL.RawQuery = testCase.query
			request.RequestURI = request.URL.RequestURI()
			request.RemoteAddr = remoteAddr
			request.TLS = &tls.ConnectionState{ServerName: testCase.serverName}
			for name, value := range testCase.headers {
				request.Header.Set(name, value)
			}
			for name, lines := range testCase.lines {
				for _, line := range lines {
					request.Header.Add(name, line)
				}
			}

			logged, status := captureVhostLog(t, vhostMux, request)

			if status != http.StatusMisdirectedRequest {
				t.Fatalf("status = %d, want %d", status, http.StatusMisdirectedRequest)
			}
			if len(logged) == 0 {
				t.Fatal("nothing was logged")
			}
			if len(logged) > logLimit {
				t.Fatalf("logged %d bytes, want at most %d", len(logged), logLimit)
			}
			for _, want := range testCase.wantLogged {
				if !strings.Contains(logged, want) {
					t.Errorf("the log does not contain %q:\n%s", want, logged)
				}
			}
			for _, absent := range testCase.wantAbsent {
				if strings.Contains(logged, absent) {
					t.Errorf("the log contains %q:\n%s", absent, logged)
				}
			}
		})
	}
}

// The summary is the one written for a misdirected request; what a vhost serves
// is logged as it was, in full.
//
//nolint:paralleltest // Swaps the default logger, which is global, to read what is logged.
func TestVhostMux_ServedRequestLogKeepsHeaders(t *testing.T) {
	const servedHost = "open.home.arpa"

	vhostMux := &VhostMux{
		HostToSpecification: map[string]*VhostMuxSpecification{
			servedHost: {Mux: http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				slog.WarnContext(request.Context(), "served")
				responseWriter.WriteHeader(http.StatusNoContent)
			})},
		},
	}
	// The vhost's own logging reads the HTTP context of the mux it is served
	// by; the vhost mux's DoneCallback reads the one it made itself.
	vhostMux.DoneCallback = func(ctx context.Context) {
		slog.WarnContext(ctx, "done")
	}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+servedHost+"/", nil)
	request.Host = servedHost
	request.RemoteAddr = "192.0.2.10:54321"
	request.TLS = &tls.ConnectionState{ServerName: servedHost}
	request.Header.Set("X-Marker", "kept-in-full")

	logged, status := captureVhostLog(t, vhostMux, request)
	if status != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", status, http.StatusNoContent)
	}
	if !strings.Contains(logged, "kept-in-full") {
		t.Fatalf("expected the served request's headers in the log:\n%s", logged)
	}
}

func TestTruncateValue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		value    string
		limit    int
		expected string
	}{
		{name: "shorter than the limit", value: "abc", limit: 5, expected: "abc"},
		{name: "as long as the limit", value: "abcde", limit: 5, expected: "abcde"},
		{name: "longer than the limit", value: "abcdef", limit: 5, expected: "abcde... [truncated, 6 bytes in full]"},
		{name: "cut inside a character", value: "abcdé", limit: 5, expected: "abcd... [truncated, 6 bytes in full]"},
		{name: "empty", value: "", limit: 5, expected: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := truncateValue(testCase.value, testCase.limit); got != testCase.expected {
				t.Fatalf("truncateValue(%q, %d) = %q, want %q", testCase.value, testCase.limit, got, testCase.expected)
			}
		})
	}
}
