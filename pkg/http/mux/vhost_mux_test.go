package mux

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/forwarded_headers"
)

func vhostRequest(t *testing.T, host string) *http.Request {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/path", nil)
	request.Host = host
	return request
}

func TestVhostMuxHandleRequest(t *testing.T) {
	t.Parallel()

	t.Run("nil vhost mux", func(t *testing.T) {
		t.Parallel()
		_, responseError := vhostMuxHandleRequest(nil, vhostRequest(t, "a.example.com"), httptest.NewRecorder())
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})

	t.Run("nil request", func(t *testing.T) {
		t.Parallel()
		_, responseError := vhostMuxHandleRequest(&VhostMux{}, nil, httptest.NewRecorder())
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})

	t.Run("nil host map", func(t *testing.T) {
		t.Parallel()
		_, responseError := vhostMuxHandleRequest(&VhostMux{}, vhostRequest(t, "a.example.com"), httptest.NewRecorder())
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})

	t.Run("unknown host is 421", func(t *testing.T) {
		t.Parallel()
		vhostMux := &VhostMux{HostToSpecification: map[string]*VhostMuxSpecification{}}
		_, responseError := vhostMuxHandleRequest(vhostMux, vhostRequest(t, "unknown.example.com"), httptest.NewRecorder())
		if responseError == nil || responseError.ProblemDetail == nil || responseError.ProblemDetail.Status != http.StatusMisdirectedRequest {
			t.Fatalf("expected 421, got %#v", responseError)
		}
	})

	t.Run("nil specification", func(t *testing.T) {
		t.Parallel()
		vhostMux := &VhostMux{HostToSpecification: map[string]*VhostMuxSpecification{"a.example.com": nil}}
		_, responseError := vhostMuxHandleRequest(vhostMux, vhostRequest(t, "a.example.com"), httptest.NewRecorder())
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})

	t.Run("redirect", func(t *testing.T) {
		t.Parallel()
		vhostMux := &VhostMux{HostToSpecification: map[string]*VhostMuxSpecification{
			"r.example.com": {RedirectTo: "https://new.example.com"},
		}}
		response, responseError := vhostMuxHandleRequest(vhostMux, vhostRequest(t, "r.example.com"), httptest.NewRecorder())
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if response == nil || response.StatusCode != http.StatusMovedPermanently {
			t.Fatalf("expected a 301, got %#v", response)
		}
		var location string
		for _, header := range response.Headers {
			if header != nil && header.Name == "Location" {
				location = header.Value
			}
		}
		if !strings.HasPrefix(location, "https://new.example.com") {
			t.Fatalf("unexpected Location: %q", location)
		}
	})

	t.Run("delegates to the inner mux", func(t *testing.T) {
		t.Parallel()
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		})
		vhostMux := &VhostMux{HostToSpecification: map[string]*VhostMuxSpecification{
			"m.example.com": {Mux: handler},
		}}
		recorder := httptest.NewRecorder()
		response, responseError := vhostMuxHandleRequest(vhostMux, vhostRequest(t, "m.example.com"), recorder)
		if responseError != nil || response != nil {
			t.Fatalf("expected the inner mux to handle it, got response %#v error %#v", response, responseError)
		}
		if recorder.Code != http.StatusTeapot {
			t.Fatalf("expected the inner mux to write 418, got %d", recorder.Code)
		}
	})

	t.Run("unusable specification", func(t *testing.T) {
		t.Parallel()
		vhostMux := &VhostMux{HostToSpecification: map[string]*VhostMuxSpecification{"u.example.com": {}}}
		_, responseError := vhostMuxHandleRequest(vhostMux, vhostRequest(t, "u.example.com"), httptest.NewRecorder())
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})
}

func TestVhostMux_ServeHTTP(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("inner"))
	})
	vhostMux := &VhostMux{HostToSpecification: map[string]*VhostMuxSpecification{
		"app.example.com": {Mux: handler},
	}}

	recorder := httptest.NewRecorder()
	vhostMux.ServeHTTP(recorder, vhostRequest(t, "app.example.com"))

	if recorder.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", recorder.Code)
	}
	if recorder.Body.String() != "inner" {
		t.Fatalf("body = %q, want inner", recorder.Body.String())
	}
}

func TestVhostMux_PatchHttpServer(t *testing.T) {
	t.Parallel()

	t.Run("nil server is a no-op", func(t *testing.T) {
		t.Parallel()
		(&VhostMux{}).PatchHttpServer(nil)
	})

	t.Run("patches handler and certificate resolver", func(t *testing.T) {
		t.Parallel()

		certificate := &tls.Certificate{}
		vhostMux := &VhostMux{HostToSpecification: map[string]*VhostMuxSpecification{
			"secure.example.com": {Certificate: certificate},
		}}
		server := &http.Server{ReadHeaderTimeout: time.Second}
		vhostMux.PatchHttpServer(server)

		if server.Handler == nil {
			t.Error("expected the server handler to be set")
		}
		if server.TLSConfig == nil || server.TLSConfig.GetCertificate == nil {
			t.Fatal("expected GetCertificate to be set")
		}

		got, err := server.TLSConfig.GetCertificate(&tls.ClientHelloInfo{ServerName: "secure.example.com"})
		if err != nil {
			t.Fatalf("get certificate: %v", err)
		}
		if got != certificate {
			t.Error("expected the configured certificate")
		}

		if got, err := server.TLSConfig.GetCertificate(nil); err != nil || got != nil {
			t.Errorf("nil client hello: got %#v err %v", got, err)
		}

		if got, err := server.TLSConfig.GetCertificate(&tls.ClientHelloInfo{ServerName: "unknown.example.com"}); err != nil || got != nil {
			t.Errorf("unknown host: got %#v err %v", got, err)
		}
	})

	t.Run("nil host map yields a certificate error", func(t *testing.T) {
		t.Parallel()
		server := &http.Server{ReadHeaderTimeout: time.Second}
		(&VhostMux{}).PatchHttpServer(server)
		if _, err := server.TLSConfig.GetCertificate(&tls.ClientHelloInfo{ServerName: "x"}); err == nil {
			t.Fatal("expected an error for a nil host map")
		}
	})
}

func TestVhostMuxHandleRequest_TrustForwardedHost(t *testing.T) {
	t.Parallel()

	// The shape Firebase Hosting produces: Host is the address it dialled, and
	// the name the client asked for survives only in the forwarded headers.
	const servedHost = "wiki.vvvp.se"
	const proxyHost = "vvvp-wiki-abcdef.run.app"

	testCases := []struct {
		name           string
		headers        map[string]string
		trustForwarded bool
		expectServed   bool
	}{
		{
			name:           "untrusted refuses the proxy's host",
			headers:        map[string]string{"X-Forwarded-Host": servedHost},
			trustForwarded: false,
			expectServed:   false,
		},
		{
			name:           "trusted serves on X-Forwarded-Host",
			headers:        map[string]string{"X-Forwarded-Host": servedHost},
			trustForwarded: true,
			expectServed:   true,
		},
		{
			name:           "trusted serves on Forwarded",
			headers:        map[string]string{"Forwarded": "host=" + servedHost},
			trustForwarded: true,
			expectServed:   true,
		},
		{
			name:           "trusted still refuses a host it does not answer for",
			headers:        map[string]string{"X-Forwarded-Host": "elsewhere.example.com"},
			trustForwarded: true,
			expectServed:   false,
		},
		{
			name:           "trusted with no forwarded headers refuses the proxy's host",
			headers:        map[string]string{},
			trustForwarded: true,
			expectServed:   false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var served bool
			vhostMux := &VhostMux{
				HostToSpecification: map[string]*VhostMuxSpecification{
					servedHost: {Mux: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
						served = true
					})},
				},
				TrustForwardedHost: testCase.trustForwarded,
			}

			request := vhostRequest(t, proxyHost)
			for name, value := range testCase.headers {
				request.Header.Set(name, value)
			}

			_, responseError := vhostMuxHandleRequest(vhostMux, request, httptest.NewRecorder())

			if testCase.expectServed {
				if !served {
					t.Fatalf("expected the request to be served, got %#v", responseError)
				}
				return
			}

			if served {
				t.Fatal("expected the request to be refused, but it was served")
			}
			if responseError == nil || responseError.ProblemDetail == nil ||
				responseError.ProblemDetail.Status != http.StatusMisdirectedRequest {
				t.Fatalf("expected 421, got %#v", responseError)
			}
		})
	}
}

// With trusted proxies and a trusted X-Forwarded-For, the forwarded host is
// believed only of a request the proxy directly in front recorded as coming
// from one -- through Firebase Hosting, Google's front end appends the edge's
// address, and sent to the run.app address directly, the caller's -- and only
// from X-Forwarded-Host. Without a trusted X-Forwarded-For it is believed of
// none.
func TestVhostMuxHandleRequest_TrustedProxies(t *testing.T) {
	t.Parallel()

	const servedHost = "films.vvvp.se"
	const proxyHost = "vvvp-films-abcdef.run.app"

	firebaseEdges := []netip.Prefix{netip.MustParsePrefix("66.249.93.0/24")}

	testCases := []struct {
		name               string
		lines              map[string][]string
		serverName         string
		trustForwardedHost bool
		trustForwardedFor  bool
		trustedProxies     []netip.Prefix
		expectedAuthority  string
	}{
		{
			name:               "no trusted proxies, run.app directly, as before",
			lines:              map[string][]string{"X-Forwarded-Host": {servedHost}, "X-Forwarded-For": {"192.0.2.21,203.0.113.9"}},
			trustForwardedHost: true,
			expectedAuthority:  servedHost,
		},
		{
			name:               "through Firebase Hosting",
			lines:              map[string][]string{"X-Forwarded-Host": {servedHost}, "X-Forwarded-For": {"198.51.100.7,66.249.93.77"}},
			trustForwardedHost: true,
			trustForwardedFor:  true,
			trustedProxies:     firebaseEdges,
			expectedAuthority:  servedHost,
		},
		{
			// Firebase Hosting passes a client's Forwarded through unchanged, so it is not read
			// where the trusted proxies gate the forwarded host.
			name:               "through Firebase Hosting, on Forwarded alone",
			lines:              map[string][]string{"Forwarded": {"host=" + servedHost}, "X-Forwarded-For": {"198.51.100.7,66.249.93.133"}},
			trustForwardedHost: true,
			trustForwardedFor:  true,
			trustedProxies:     firebaseEdges,
		},
		{
			name: "through Firebase Hosting, a client's Forwarded behind X-Forwarded-Host",
			lines: map[string][]string{
				"Forwarded":        {"host=" + proxyHost},
				"X-Forwarded-Host": {servedHost},
				"X-Forwarded-For":  {"198.51.100.7,66.249.93.133"},
			},
			trustForwardedHost: true,
			trustForwardedFor:  true,
			trustedProxies:     firebaseEdges,
			expectedAuthority:  servedHost,
		},
		{
			name:               "through Firebase Hosting, X-Forwarded-For not trusted",
			lines:              map[string][]string{"X-Forwarded-Host": {servedHost}, "X-Forwarded-For": {"198.51.100.7,66.249.93.77"}},
			trustForwardedHost: true,
			trustedProxies:     firebaseEdges,
		},
		{
			name:               "run.app directly with X-Forwarded-Host",
			lines:              map[string][]string{"X-Forwarded-Host": {servedHost}, "X-Forwarded-For": {"203.0.113.9"}},
			trustForwardedHost: true,
			trustForwardedFor:  true,
			trustedProxies:     firebaseEdges,
		},
		{
			name: "run.app directly with Forwarded",
			lines: map[string][]string{
				"Forwarded":       {"for=192.0.2.21;host=" + servedHost + `, for="203.0.113.9"`},
				"X-Forwarded-For": {"192.0.2.21,203.0.113.9"},
			},
			trustForwardedHost: true,
			trustForwardedFor:  true,
			trustedProxies:     firebaseEdges,
		},
		{
			name:               "run.app directly, naming an edge",
			lines:              map[string][]string{"X-Forwarded-Host": {servedHost}, "X-Forwarded-For": {"198.51.100.7,66.249.93.77,203.0.113.9"}},
			trustForwardedHost: true,
			trustForwardedFor:  true,
			trustedProxies:     firebaseEdges,
		},
		{
			name:               "run.app directly, no X-Forwarded-For",
			lines:              map[string][]string{"X-Forwarded-Host": {servedHost}},
			trustForwardedHost: true,
			trustForwardedFor:  true,
			trustedProxies:     firebaseEdges,
		},
		{
			name:           "through Firebase Hosting, the forwarded host not trusted",
			lines:          map[string][]string{"X-Forwarded-Host": {servedHost}, "X-Forwarded-For": {"198.51.100.7,66.249.93.77"}},
			trustedProxies: firebaseEdges,
		},
		{
			name:               "through Firebase Hosting, a server name of its own",
			lines:              map[string][]string{"X-Forwarded-Host": {servedHost}, "X-Forwarded-For": {"198.51.100.7,66.249.93.77"}},
			serverName:         proxyHost,
			trustForwardedHost: true,
			trustForwardedFor:  true,
			trustedProxies:     firebaseEdges,
			expectedAuthority:  servedHost,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var served bool
			var authority string
			vhostMux := &VhostMux{
				HostToSpecification: map[string]*VhostMuxSpecification{
					servedHost: {Mux: http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
						served = true
						authority, _ = forwarded_headers.AuthorityFromContext(request.Context())
					})},
				},
				TrustForwardedHost: testCase.trustForwardedHost,
			}
			vhostMux.TrustForwardedFor = testCase.trustForwardedFor
			vhostMux.TrustedProxies = testCase.trustedProxies

			request := vhostRequest(t, proxyHost)
			if testCase.serverName != "" {
				request.TLS = &tls.ConnectionState{ServerName: testCase.serverName}
			}
			for name, lines := range testCase.lines {
				for _, line := range lines {
					request.Header.Add(name, line)
				}
			}

			_, responseError := vhostMuxHandleRequest(vhostMux, request, httptest.NewRecorder())

			if testCase.expectedAuthority != "" {
				if !served {
					t.Fatalf("expected the request to be served, got %#v", responseError)
				}
				if authority != testCase.expectedAuthority {
					t.Fatalf("authority = %q, want %q", authority, testCase.expectedAuthority)
				}
				return
			}

			if served {
				t.Fatal("expected the request to be refused, but it was served")
			}
			if responseError == nil || responseError.ProblemDetail == nil ||
				responseError.ProblemDetail.Status != http.StatusMisdirectedRequest {
				t.Fatalf("expected 421, got %#v", responseError)
			}
		})
	}
}

// A request the forwarded host is not believed of is routed on its own Host,
// and held to the server name its connection was opened with, as where the
// forwarded host is not trusted at all.
func TestVhostMuxHandleRequest_TrustedProxiesServerName(t *testing.T) {
	t.Parallel()

	const servedHost = "films.vvvp.se"
	const otherHost = "restic.home.arpa"

	testCases := []struct {
		name          string
		forwardedFor  string
		expectedServe bool
	}{
		{name: "through a trusted proxy, the server name is not the host routed on", forwardedFor: "198.51.100.7,66.249.93.77", expectedServe: true},
		{name: "around the trusted proxies, the server name has to be the host", forwardedFor: "198.51.100.7,203.0.113.9", expectedServe: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var served bool
			vhostMux := &VhostMux{
				HostToSpecification: map[string]*VhostMuxSpecification{
					servedHost: {Mux: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { served = true })},
				},
				TrustForwardedHost: true,
			}
			vhostMux.TrustForwardedFor = true
			vhostMux.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("66.249.93.0/24")}

			request := vhostRequest(t, servedHost)
			request.TLS = &tls.ConnectionState{ServerName: otherHost}
			request.Header.Set("X-Forwarded-Host", servedHost)
			request.Header.Set("X-Forwarded-For", testCase.forwardedFor)

			_, responseError := vhostMuxHandleRequest(vhostMux, request, httptest.NewRecorder())

			if served != testCase.expectedServe {
				t.Fatalf("served = %t, want %t (%#v)", served, testCase.expectedServe, responseError)
			}
			if !testCase.expectedServe && (responseError == nil || responseError.ProblemDetail == nil ||
				responseError.ProblemDetail.Status != http.StatusMisdirectedRequest) {
				t.Fatalf("expected 421, got %#v", responseError)
			}
		})
	}
}

// The forwarded host is read from X-Forwarded-Host ahead of Forwarded: as
// measured, Firebase Hosting overwrites X-Forwarded-Host and passes a client's
// `Forwarded: host=` through unchanged, which a mux reading Forwarded first
// routed on. Where trusted proxies gate the forwarded host, Forwarded is not
// read for it at all.
func TestVhostMuxHandleRequest_ForwardedHostOrder(t *testing.T) {
	t.Parallel()

	const servedHost = "films.vvvp.se"
	const otherHost = "evil.example"
	const proxyHost = "vvvp-films-abcdef.run.app"

	firebaseEdges := []netip.Prefix{netip.MustParsePrefix("66.249.93.0/24")}

	testCases := []struct {
		name           string
		headers        map[string]string
		trustedProxies []netip.Prefix
		expectedHost   string
	}{
		{
			name:         "through Firebase Hosting, a client's Forwarded",
			headers:      map[string]string{"X-Forwarded-Host": servedHost, "Forwarded": "host=" + otherHost, "X-Forwarded-For": "198.51.100.7,66.249.93.77"},
			expectedHost: servedHost,
		},
		{
			name:         "reverse_proxy, both headers alike",
			headers:      map[string]string{"X-Forwarded-Host": servedHost, "Forwarded": "for=198.51.100.7;host=" + servedHost, "X-Forwarded-For": "198.51.100.7"},
			expectedHost: servedHost,
		},
		{
			name:         "Forwarded alone",
			headers:      map[string]string{"Forwarded": "host=" + otherHost},
			expectedHost: otherHost,
		},
		{
			name:           "gated, a client's Forwarded",
			headers:        map[string]string{"X-Forwarded-Host": servedHost, "Forwarded": "host=" + otherHost, "X-Forwarded-For": "198.51.100.7,66.249.93.77"},
			trustedProxies: firebaseEdges,
			expectedHost:   servedHost,
		},
		{
			name:           "gated, Forwarded alone",
			headers:        map[string]string{"Forwarded": "host=" + otherHost, "X-Forwarded-For": "198.51.100.7,66.249.93.77"},
			trustedProxies: firebaseEdges,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var servedAs string
			serve := func(host string) http.Handler {
				return http.HandlerFunc(func(http.ResponseWriter, *http.Request) { servedAs = host })
			}
			vhostMux := &VhostMux{
				HostToSpecification: map[string]*VhostMuxSpecification{
					servedHost: {Mux: serve(servedHost)},
					otherHost:  {Mux: serve(otherHost)},
				},
				TrustForwardedHost: true,
			}
			vhostMux.TrustForwardedFor = true
			vhostMux.TrustedProxies = testCase.trustedProxies

			request := vhostRequest(t, proxyHost)
			for name, value := range testCase.headers {
				request.Header.Set(name, value)
			}

			_, responseError := vhostMuxHandleRequest(vhostMux, request, httptest.NewRecorder())

			if servedAs != testCase.expectedHost {
				t.Fatalf("served as %q, want %q (%#v)", servedAs, testCase.expectedHost, responseError)
			}
			if testCase.expectedHost == "" && (responseError == nil || responseError.ProblemDetail == nil ||
				responseError.ProblemDetail.Status != http.StatusMisdirectedRequest) {
				t.Fatalf("expected 421, got %#v", responseError)
			}
		})
	}
}

func TestVhostMuxHandleRequest_ServerNameMatchesHost(t *testing.T) {
	t.Parallel()

	const servedHost = "upload-logs.home.arpa"
	const otherHost = "restic.home.arpa"

	testCases := []struct {
		name           string
		serverName     string
		host           string
		tls            bool
		trustForwarded bool
		expectServed   bool
	}{
		{
			name:         "the server name and the host agree",
			serverName:   servedHost,
			host:         servedHost,
			tls:          true,
			expectServed: true,
		},
		{
			name:         "case and port do not make them disagree",
			serverName:   "Upload-Logs.Home.Arpa",
			host:         servedHost + ":443",
			tls:          true,
			expectServed: true,
		},
		{
			// Handshaking as a vhost that asks nothing of the client and then
			// addressing one that does is what the check exists for.
			name:         "another vhost's server name is refused",
			serverName:   otherHost,
			host:         servedHost,
			tls:          true,
			expectServed: false,
		},
		{
			name:         "no server name is refused",
			serverName:   "",
			host:         servedHost,
			tls:          true,
			expectServed: false,
		},
		{
			name:         "a plaintext request has no server name to disagree with",
			host:         servedHost,
			tls:          false,
			expectServed: true,
		},
		{
			// What follows the host would otherwise reach the vhost's handler,
			// and through it whatever repeats the authority upstream.
			name:         "a port that is not a number is refused",
			serverName:   servedHost,
			host:         servedHost + ":443,for=127.0.0.1;by=x",
			tls:          true,
			expectServed: false,
		},
		{
			name:         "so is one that is not a number in plaintext",
			host:         servedHost + ":x",
			tls:          false,
			expectServed: false,
		},
		{
			name:           "and one that is not a number where the forwarded host is trusted",
			host:           servedHost + ":443;x",
			tls:            true,
			trustForwarded: true,
			expectServed:   false,
		},
		{
			name:         "an empty port is a port",
			serverName:   servedHost,
			host:         servedHost + ":",
			tls:          true,
			expectServed: true,
		},
		{
			// With the forwarded headers trusted, TLS was terminated in front
			// and the host routed on is not the connection's.
			name:           "trusting the forwarded host exempts the request",
			serverName:     otherHost,
			host:           servedHost,
			tls:            true,
			trustForwarded: true,
			expectServed:   true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var served bool
			vhostMux := &VhostMux{
				HostToSpecification: map[string]*VhostMuxSpecification{
					servedHost: {Mux: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
						served = true
					})},
					otherHost: {Mux: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
						t.Error("the request reached the wrong vhost")
					})},
				},
				TrustForwardedHost: testCase.trustForwarded,
			}

			request := vhostRequest(t, testCase.host)
			if testCase.tls {
				request.TLS = &tls.ConnectionState{ServerName: testCase.serverName}
			}

			_, responseError := vhostMuxHandleRequest(vhostMux, request, httptest.NewRecorder())

			if testCase.expectServed {
				if !served {
					t.Fatalf("expected the request to be served, got %#v", responseError)
				}
				return
			}

			if served {
				t.Fatal("expected the request to be refused, but it was served")
			}
			if responseError == nil || responseError.ProblemDetail == nil ||
				responseError.ProblemDetail.Status != http.StatusMisdirectedRequest {
				t.Fatalf("expected 421, got %#v", responseError)
			}
		})
	}
}

func TestVhostMuxHandleRequest_CarriesAuthority(t *testing.T) {
	t.Parallel()

	const servedHost = "wiki.vvvp.se"

	t.Run("the resolved authority reaches the served mux", func(t *testing.T) {
		t.Parallel()

		var seen string
		vhostMux := &VhostMux{
			HostToSpecification: map[string]*VhostMuxSpecification{
				servedHost: {Mux: http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
					seen, _ = forwarded_headers.AuthorityFromContext(request.Context())
				})},
			},
			TrustForwardedHost: true,
		}

		request := vhostRequest(t, "vvvp-wiki-abcdef.run.app")
		request.Header.Set("X-Forwarded-Host", servedHost)

		if _, responseError := vhostMuxHandleRequest(vhostMux, request, httptest.NewRecorder()); responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if seen != servedHost {
			t.Fatalf("got %q, want %q", seen, servedHost)
		}
	})

	t.Run("the port survives for a URL built from it", func(t *testing.T) {
		t.Parallel()

		var seen string
		vhostMux := &VhostMux{
			HostToSpecification: map[string]*VhostMuxSpecification{
				"localhost": {Mux: http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
					seen, _ = forwarded_headers.AuthorityFromContext(request.Context())
				})},
			},
		}

		if _, responseError := vhostMuxHandleRequest(vhostMux, vhostRequest(t, "localhost:8080"), httptest.NewRecorder()); responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if seen != "localhost:8080" {
			t.Fatalf("got %q, want %q", seen, "localhost:8080")
		}
	})
}
