package public_fetch

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
	"github.com/altshiftab/utils_go/pkg/net/outbound_guard"
)

const (
	testUserAgent = "public_fetch test (downloads test files)"
	testMaxBytes  = 1024
)

// testServers are two TLS servers on loopback: the one the tests' fetchers may connect to, and another,
// which stands for an internal host a redirect leads to.
type testServers struct {
	allowed *httptest.Server
	other   *httptest.Server
	tls     *tls.Config
}

func newTestServers(t *testing.T) *testServers {
	t.Helper()

	other := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("internal"))
	}))
	t.Cleanup(other.Close)

	mux := http.NewServeMux()
	mux.HandleFunc("/files/plain.bin", func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("User-Agent") != testUserAgent {
			http.Error(writer, "wrong user agent", http.StatusBadRequest)
			return
		}
		_, _ = writer.Write([]byte("plain"))
	})
	mux.HandleFunc("/download", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Disposition", `attachment; filename="C:\\Users\\x\\Manual (de-en).pdf"`)
		_, _ = writer.Write([]byte("named"))
	})
	mux.HandleFunc("/", func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("root"))
	})
	mux.HandleFunc("/exact", func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(bytes.Repeat([]byte("a"), testMaxBytes))
	})
	mux.HandleFunc("/big", func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(bytes.Repeat([]byte("a"), testMaxBytes+1))
	})
	mux.HandleFunc("/big-unannounced", func(writer http.ResponseWriter, _ *http.Request) {
		// Flushed in parts, so that no Content-Length is sent and the limit is met while reading.
		for range 4 {
			_, _ = writer.Write(bytes.Repeat([]byte("a"), testMaxBytes/2))
			writer.(http.Flusher).Flush()
		}
	})
	mux.HandleFunc("/missing", func(writer http.ResponseWriter, _ *http.Request) {
		http.NotFound(writer, nil)
	})
	mux.HandleFunc("/to-http", func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "http://example.invalid/file", http.StatusFound)
	})
	mux.HandleFunc("/to-internal", func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, other.URL+"/file", http.StatusFound)
	})
	mux.HandleFunc("/to-user-info", func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "https://user:secret@example.invalid/file", http.StatusFound)
	})
	mux.HandleFunc("/loop", func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/loop", http.StatusFound)
	})
	mux.HandleFunc("/to-plain", func(writer http.ResponseWriter, request *http.Request) {
		http.SetCookie(writer, &http.Cookie{Name: "session", Value: "x", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		http.Redirect(writer, request, "/no-cookie/redirected.pdf", http.StatusFound)
	})
	mux.HandleFunc("/no-cookie/redirected.pdf", func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Cookie") != "" {
			http.Error(writer, "a cookie was sent", http.StatusBadRequest)
			return
		}
		_, _ = writer.Write([]byte("redirected"))
	})
	mux.HandleFunc("/slow-headers", func(writer http.ResponseWriter, request *http.Request) {
		select {
		case <-request.Context().Done():
		case <-time.After(5 * time.Second):
		}
		_, _ = writer.Write([]byte("late"))
	})
	mux.HandleFunc("/slow-body", func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("start"))
		writer.(http.Flusher).Flush()
		select {
		case <-request.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})

	allowed := httptest.NewTLSServer(mux)
	t.Cleanup(allowed.Close)

	roots := x509.NewCertPool()
	roots.AddCert(allowed.Certificate())
	roots.AddCert(other.Certificate())
	return &testServers{allowed: allowed, other: other, tls: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
}

// guardAllowing is the guard, but for a connection to the address, which it lets through.
func guardAllowing(address string) func(string, string, syscall.RawConn) error {
	return func(network string, dialled string, rawConn syscall.RawConn) error {
		if dialled == address {
			return nil
		}
		return outbound_guard.DialControl(network, dialled, rawConn)
	}
}

func (s *testServers) fetcher(t *testing.T, options ...Option) *Fetcher {
	t.Helper()
	all := append([]Option{WithDialControl(guardAllowing(s.allowed.Listener.Addr().String())), WithTlsConfig(s.tls)}, options...)
	fetcher, err := New(testUserAgent, testMaxBytes, all...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return fetcher
}

func TestFetch(t *testing.T) {
	t.Parallel()

	servers := newTestServers(t)
	fetcher := servers.fetcher(t)
	guarded, err := New(testUserAgent, testMaxBytes, WithTlsConfig(servers.tls))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	quick := servers.fetcher(t, WithTimeout(200*time.Millisecond))
	base := servers.allowed.URL

	testCases := []struct {
		name     string
		fetcher  *Fetcher
		address  string
		wantData string
		wantName string
		wantUrl  string
		// wantCause is the refusal's cause; nil is a download that succeeds.
		wantCause error
	}{
		{name: "a file, named by its path", fetcher: fetcher, address: base + "/files/plain.bin", wantData: "plain", wantName: "plain.bin", wantUrl: base + "/files/plain.bin"},
		{name: "a file named by its Content-Disposition, the last part of the path it gives", fetcher: fetcher, address: base + "/download", wantData: "named", wantName: "Manual (de-en).pdf", wantUrl: base + "/download"},
		{name: "a file of no name", fetcher: fetcher, address: base + "/", wantData: "root", wantName: "", wantUrl: base + "/"},
		{name: "a fragment is not sent", fetcher: fetcher, address: base + "/files/plain.bin#page=2", wantData: "plain", wantName: "plain.bin", wantUrl: base + "/files/plain.bin"},
		{name: "a file of the most bytes", fetcher: fetcher, address: base + "/exact", wantData: strings.Repeat("a", testMaxBytes), wantName: "exact", wantUrl: base + "/exact"},
		{name: "a redirect, with no cookie kept, named by where it led", fetcher: fetcher, address: base + "/to-plain", wantData: "redirected", wantName: "redirected.pdf", wantUrl: base + "/no-cookie/redirected.pdf"},
		{name: "a loopback host, refused by the guard", fetcher: guarded, address: base + "/files/plain.bin", wantCause: outbound_guard.ErrNotPublic},
		{name: "a redirect to an internal host", fetcher: fetcher, address: base + "/to-internal", wantCause: outbound_guard.ErrNotPublic},
		{name: "http", fetcher: fetcher, address: strings.Replace(base, "https://", "http://", 1) + "/files/plain.bin", wantCause: ErrNotHttps},
		{name: "a redirect to http", fetcher: fetcher, address: base + "/to-http", wantCause: ErrNotHttps},
		{name: "a user name and a password", fetcher: fetcher, address: strings.Replace(base, "https://", "https://user:secret@", 1) + "/files/plain.bin", wantCause: ErrUserInfo},
		{name: "a redirect to a user name and a password", fetcher: fetcher, address: base + "/to-user-info", wantCause: ErrUserInfo},
		{name: "too many redirects", fetcher: fetcher, address: base + "/loop", wantCause: ErrTooManyRedirects},
		{name: "no URL", fetcher: fetcher, address: "a manual", wantCause: ErrInvalidUrl},
		{name: "no host", fetcher: fetcher, address: "https:///file", wantCause: ErrInvalidUrl},
		{name: "a file too large, announced", fetcher: fetcher, address: base + "/big", wantCause: altshiftHttpErrors.ErrResponseBodyTooLarge},
		{name: "a file too large, found reading it", fetcher: fetcher, address: base + "/big-unannounced", wantCause: altshiftHttpErrors.ErrResponseBodyTooLarge},
		{name: "a 404", fetcher: fetcher, address: base + "/missing", wantCause: altshiftHttpErrors.ErrNon2xxStatusCode},
		{name: "a site too slow to answer", fetcher: quick, address: base + "/slow-headers", wantCause: ErrUnreachable},
		{name: "a site too slow to send the file", fetcher: quick, address: base + "/slow-body", wantCause: ErrUnreachable},
		{name: "a name that does not resolve", fetcher: fetcher, address: "https://nothing.invalid/file", wantCause: ErrUnreachable},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			file, err := testCase.fetcher.Fetch(context.Background(), testCase.address)
			if testCase.wantCause == nil {
				if err != nil {
					t.Fatalf("Fetch: %v", err)
				}
				if string(file.Data) != testCase.wantData || file.Name != testCase.wantName || file.Url != testCase.wantUrl {
					t.Fatalf("Fetch = %q named %q from %q, want %q named %q from %q",
						file.Data, file.Name, file.Url, testCase.wantData, testCase.wantName, testCase.wantUrl)
				}
				return
			}
			if file != nil {
				t.Fatalf("Fetch = %q, want a refusal", file.Data)
			}
			refusal, ok := errors.AsType[*RefusalError](err)
			if !ok || refusal.Message == "" {
				t.Fatalf("Fetch error = %v, want a *RefusalError with a message", err)
			}
			if !errors.Is(err, testCase.wantCause) {
				t.Fatalf("Fetch error = %v, want one of %v", err, testCase.wantCause)
			}
			if !errors.Is(err, altshiftErrors.ErrValidationError) {
				t.Fatalf("Fetch error = %v, want a validation error", err)
			}
		})
	}
}

func TestFetchRedirectLimit(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		http.Redirect(writer, request, "/again", http.StatusFound)
	}))
	t.Cleanup(server.Close)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())

	const maxRedirects = 2
	fetcher, err := New(testUserAgent, testMaxBytes, WithMaxRedirects(maxRedirects),
		WithDialControl(guardAllowing(server.Listener.Addr().String())), WithTlsConfig(&tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := fetcher.Fetch(context.Background(), server.URL+"/start"); !errors.Is(err, ErrTooManyRedirects) {
		t.Fatalf("Fetch error = %v, want %v", err, ErrTooManyRedirects)
	}
	// The address and the redirects followed; the one past them is refused before it is requested.
	if got := requests.Load(); got != maxRedirects+1 {
		t.Fatalf("requests = %d, want %d", got, maxRedirects+1)
	}
}

// TestFetchOwnTransport sets on the process-wide default transport what would take an https connection
// past the guard, and expects the guard all the same: the fetcher's transport is its own. It does not run in
// parallel, the default transport being every test's.
//
//nolint:paralleltest // it changes http.DefaultTransport, which it puts back
func TestFetchOwnTransport(t *testing.T) {
	servers := newTestServers(t)
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok || defaultTransport == nil {
		t.Skip("the default transport is not an *http.Transport")
	}
	unguarded := &tls.Dialer{Config: servers.tls}
	defaultTransport.DialTLSContext = unguarded.DialContext
	defaultTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // the test's point: none of it may reach the fetcher
	t.Cleanup(func() {
		defaultTransport.DialTLSContext = nil
		defaultTransport.TLSClientConfig = nil
	})

	fetcher, err := New(testUserAgent, testMaxBytes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := fetcher.Fetch(context.Background(), servers.allowed.URL+"/files/plain.bin"); !errors.Is(err, outbound_guard.ErrNotPublic) {
		t.Fatalf("Fetch error = %v, want %v", err, outbound_guard.ErrNotPublic)
	}
}

func TestFetchLocalFailure(t *testing.T) {
	t.Parallel()

	servers := newTestServers(t)
	fetcher, err := New(testUserAgent, testMaxBytes, WithTlsConfig(servers.tls),
		WithDialControl(func(string, string, syscall.RawConn) error { return syscall.EMFILE }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = fetcher.Fetch(context.Background(), servers.allowed.URL+"/files/plain.bin")
	if !errors.Is(err, syscall.EMFILE) {
		t.Fatalf("Fetch error = %v, want %v", err, syscall.EMFILE)
	}
	if _, ok := errors.AsType[*RefusalError](err); ok {
		t.Fatalf("Fetch error = %v, want no refusal: the service ran out of files, the site did nothing wrong", err)
	}
}

func TestFetchContextEnded(t *testing.T) {
	t.Parallel()

	servers := newTestServers(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := servers.fetcher(t).Fetch(ctx, servers.allowed.URL+"/files/plain.bin")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Fetch error = %v, want the context's", err)
	}
	if _, ok := errors.AsType[*RefusalError](err); ok {
		t.Fatalf("Fetch error = %v, want no refusal: the caller left, the site did nothing wrong", err)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		userAgent string
		maxBytes  int64
		timeout   *time.Duration
		wantErr   bool
	}{
		{name: "a user agent and a limit", userAgent: testUserAgent, maxBytes: 1},
		{name: "no user agent", userAgent: " ", maxBytes: 1, wantErr: true},
		{name: "no limit", userAgent: testUserAgent, maxBytes: 0, wantErr: true},
		{name: "a limit one byte past which cannot be read", userAgent: testUserAgent, maxBytes: math.MaxInt64, wantErr: true},
		{name: "a timeout of nothing, which would be none", userAgent: testUserAgent, maxBytes: 1, timeout: new(time.Duration(0)), wantErr: true},
		{name: "a timeout below nothing", userAgent: testUserAgent, maxBytes: 1, timeout: new(-time.Second), wantErr: true},
		{name: "a timeout", userAgent: testUserAgent, maxBytes: 1, timeout: new(time.Second)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var options []Option
			if testCase.timeout != nil {
				options = append(options, WithTimeout(*testCase.timeout))
			}
			fetcher, err := New(testCase.userAgent, testCase.maxBytes, options...)
			if (err != nil) != testCase.wantErr || (fetcher == nil) != testCase.wantErr {
				t.Fatalf("New = %v, %v; want an error: %t", fetcher, err, testCase.wantErr)
			}
		})
	}
}

func TestNameOf(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		disposition string
		path        string
		want        string
	}{
		{name: "the disposition's filename", disposition: `attachment; filename="manual.pdf"`, path: "/x/y.bin", want: "manual.pdf"},
		{name: "an encoded filename", disposition: `attachment; filename*=UTF-8''Br%C3%A4da.pdf`, path: "/x", want: "Bräda.pdf"},
		{name: "a filename of a path", disposition: `attachment; filename="../../etc/passwd"`, path: "/x", want: "passwd"},
		{name: "a disposition of no filename", disposition: "inline", path: "/docs/guide.pdf", want: "guide.pdf"},
		{name: "a disposition that does not parse", disposition: `attachment; filename="`, path: "/docs/guide.pdf", want: "guide.pdf"},
		{name: "a path ending in a slash", path: "/docs/", want: "docs"},
		{name: "the root", path: "/", want: ""},
		{name: "no path", path: "", want: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			address, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com"+testCase.path, nil)
			if err != nil {
				t.Fatalf("http new request: %v", err)
			}
			if got := nameOf(testCase.disposition, address.URL); got != testCase.want {
				t.Fatalf("nameOf = %q, want %q", got, testCase.want)
			}
		})
	}
}
