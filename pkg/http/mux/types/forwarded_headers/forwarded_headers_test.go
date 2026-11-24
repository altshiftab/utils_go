package forwarded_headers

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestScheme(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		headers  map[string]string
		expected string
	}{
		{
			name:     "neither header",
			headers:  map[string]string{},
			expected: "",
		},
		{
			name:     "x-forwarded-proto only",
			headers:  map[string]string{"X-Forwarded-Proto": "https"},
			expected: "https",
		},
		{
			name:     "forwarded only",
			headers:  map[string]string{"Forwarded": "for=192.0.2.1;proto=https"},
			expected: "https",
		},
		{
			name: "forwarded wins over x-forwarded-proto",
			headers: map[string]string{
				"Forwarded":         "for=192.0.2.1;proto=https",
				"X-Forwarded-Proto": "http",
			},
			expected: "https",
		},
		{
			name:     "leftmost forwarded element wins",
			headers:  map[string]string{"Forwarded": "for=192.0.2.1;proto=https, for=192.0.2.2;proto=http"},
			expected: "https",
		},
		{
			name: "forwarded without a proto falls back",
			headers: map[string]string{
				"Forwarded":         "for=192.0.2.1",
				"X-Forwarded-Proto": "https",
			},
			expected: "https",
		},
		{
			name: "unparsable forwarded falls back",
			headers: map[string]string{
				"Forwarded":         "!!! not a forwarded header !!!",
				"X-Forwarded-Proto": "https",
			},
			expected: "https",
		},
		{
			name:     "x-forwarded-proto list takes the leftmost",
			headers:  map[string]string{"X-Forwarded-Proto": "https, http"},
			expected: "https",
		},
		{
			name:     "case is normalized",
			headers:  map[string]string{"X-Forwarded-Proto": "HTTPS"},
			expected: "https",
		},
		{
			name:     "surrounding whitespace is ignored",
			headers:  map[string]string{"X-Forwarded-Proto": "  https  "},
			expected: "https",
		},
		{
			name:     "a scheme that is not http or https is discarded",
			headers:  map[string]string{"X-Forwarded-Proto": "javascript"},
			expected: "",
		},
		{
			name:     "a bad proto in forwarded is discarded, not carried",
			headers:  map[string]string{"Forwarded": "for=192.0.2.1;proto=javascript"},
			expected: "",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			header := http.Header{}
			for name, value := range testCase.headers {
				header.Set(name, value)
			}

			if scheme := Scheme(header); scheme != testCase.expected {
				t.Fatalf("got %q, want %q", scheme, testCase.expected)
			}
		})
	}
}

func TestHost(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		requestHost    string
		headers        map[string]string
		trustForwarded bool
		expected       string
	}{
		{
			name:        "untrusted ignores the forwarded headers entirely",
			requestHost: "service.run.app",
			headers: map[string]string{
				"Forwarded":        "host=wiki.vvvp.se",
				"X-Forwarded-Host": "wiki.vvvp.se",
			},
			trustForwarded: false,
			expected:       "service.run.app",
		},
		{
			name:           "trusted prefers Forwarded",
			requestHost:    "service.run.app",
			headers:        map[string]string{"Forwarded": "host=wiki.vvvp.se", "X-Forwarded-Host": "other.vvvp.se"},
			trustForwarded: true,
			expected:       "wiki.vvvp.se",
		},
		{
			name:           "trusted falls back to X-Forwarded-Host",
			requestHost:    "service.run.app",
			headers:        map[string]string{"X-Forwarded-Host": "wiki.vvvp.se"},
			trustForwarded: true,
			expected:       "wiki.vvvp.se",
		},
		{
			name:           "trusted with neither header keeps the request host",
			requestHost:    "service.run.app",
			headers:        map[string]string{},
			trustForwarded: true,
			expected:       "service.run.app",
		},
		{
			name:           "a port is dropped",
			requestHost:    "wiki.vvvp.se:8443",
			headers:        map[string]string{},
			trustForwarded: false,
			expected:       "wiki.vvvp.se",
		},
		{
			name:           "case is normalized",
			requestHost:    "WIKI.VVVP.SE",
			headers:        map[string]string{},
			trustForwarded: false,
			expected:       "wiki.vvvp.se",
		},
		{
			name:           "the leftmost X-Forwarded-Host entry wins",
			requestHost:    "service.run.app",
			headers:        map[string]string{"X-Forwarded-Host": "wiki.vvvp.se, proxy.internal"},
			trustForwarded: true,
			expected:       "wiki.vvvp.se",
		},
		{
			name:           "an unparsable Forwarded falls back rather than failing",
			requestHost:    "service.run.app",
			headers:        map[string]string{"Forwarded": "!!!", "X-Forwarded-Host": "wiki.vvvp.se"},
			trustForwarded: true,
			expected:       "wiki.vvvp.se",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://placeholder/", nil)
			request.Host = testCase.requestHost
			for name, value := range testCase.headers {
				request.Header.Set(name, value)
			}

			if host := Host(request, testCase.trustForwarded); host != testCase.expected {
				t.Fatalf("got %q, want %q", host, testCase.expected)
			}
		})
	}
}

func TestHost_NilRequest(t *testing.T) {
	t.Parallel()

	if host := Host(nil, true); host != "" {
		t.Fatalf("got %q, want %q", host, "")
	}
}

func TestAuthority(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		requestHost    string
		headers        map[string]string
		trustForwarded bool
		expected       string
	}{
		{
			name:           "the port is kept, unlike Host",
			requestHost:    "localhost:8080",
			trustForwarded: false,
			expected:       "localhost:8080",
		},
		{
			name:           "a forwarded authority keeps its port too",
			requestHost:    "service.run.app",
			headers:        map[string]string{"X-Forwarded-Host": "wiki.vvvp.se:8443"},
			trustForwarded: true,
			expected:       "wiki.vvvp.se:8443",
		},
		{
			name:           "untrusted keeps the request's own authority",
			requestHost:    "service.run.app",
			headers:        map[string]string{"X-Forwarded-Host": "wiki.vvvp.se"},
			trustForwarded: false,
			expected:       "service.run.app",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://placeholder/", nil)
			request.Host = testCase.requestHost
			for name, value := range testCase.headers {
				request.Header.Set(name, value)
			}

			if authority := Authority(request, testCase.trustForwarded); authority != testCase.expected {
				t.Fatalf("got %q, want %q", authority, testCase.expected)
			}
		})
	}
}

func TestAuthorityContext(t *testing.T) {
	t.Parallel()

	t.Run("round trip", func(t *testing.T) {
		t.Parallel()
		authority, ok := AuthorityFromContext(NewContext(t.Context(), "wiki.vvvp.se"))
		if !ok || authority != "wiki.vvvp.se" {
			t.Fatalf("got %q, %t; want %q, true", authority, ok, "wiki.vvvp.se")
		}
	})

	t.Run("absent", func(t *testing.T) {
		t.Parallel()
		if authority, ok := AuthorityFromContext(t.Context()); ok || authority != "" {
			t.Fatalf("got %q, %t; want %q, false", authority, ok, "")
		}
	})

	t.Run("empty is treated as absent", func(t *testing.T) {
		t.Parallel()
		if _, ok := AuthorityFromContext(NewContext(t.Context(), "")); ok {
			t.Fatal("an empty authority should read as absent")
		}
	})
}

// Both fields are list-valued, so a proxy may add a field line of its own
// rather than append to one that is there. The two forms mean the same thing
// and must be read the same way.
func TestMultipleFieldLines(t *testing.T) {
	t.Parallel()

	t.Run("a later Forwarded line is not hidden by an earlier one", func(t *testing.T) {
		t.Parallel()

		header := http.Header{}
		header.Add("Forwarded", "for=192.0.2.1")
		header.Add("Forwarded", "proto=https;host=wiki.vvvp.se")

		if scheme := Scheme(header); scheme != "https" {
			t.Fatalf("got scheme %q, want %q", scheme, "https")
		}

		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://placeholder/", nil)
		request.Host = "service.run.app"
		request.Header = header
		if host := Host(request, true); host != "wiki.vvvp.se" {
			t.Fatalf("got host %q, want %q", host, "wiki.vvvp.se")
		}
	})

	t.Run("the leftmost X-Forwarded-Proto line wins", func(t *testing.T) {
		t.Parallel()

		header := http.Header{}
		header.Add("X-Forwarded-Proto", "https")
		header.Add("X-Forwarded-Proto", "http")

		if scheme := Scheme(header); scheme != "https" {
			t.Fatalf("got %q, want %q", scheme, "https")
		}
	})

	t.Run("the leftmost X-Forwarded-Host line wins", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://placeholder/", nil)
		request.Host = "service.run.app"
		request.Header.Add("X-Forwarded-Host", "wiki.vvvp.se")
		request.Header.Add("X-Forwarded-Host", "proxy.internal")

		if host := Host(request, true); host != "wiki.vvvp.se" {
			t.Fatalf("got %q, want %q", host, "wiki.vvvp.se")
		}
	})
}

// An IPv6 literal is bracketed in an authority and bare in a name. Both spellings
// have to reach the same host, or a service configured for one is refused the other.
func TestHost_IPv6Literals(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		requestHost string
		expected    string
	}{
		{name: "bracketed with a port", requestHost: "[::1]:8080", expected: "::1"},
		{name: "bracketed without a port", requestHost: "[::1]", expected: "::1"},
		{name: "bare", requestHost: "::1", expected: "::1"},
		{name: "uppercase is folded", requestHost: "[2001:DB8::1]:443", expected: "2001:db8::1"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://placeholder/", nil)
			request.Host = testCase.requestHost

			if host := Host(request, false); host != testCase.expected {
				t.Fatalf("got %q, want %q", host, testCase.expected)
			}
		})
	}
}

// The authority keeps what a URL needs, including the brackets a bare host drops.
func TestAuthority_IPv6KeepsBrackets(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://placeholder/", nil)
	request.Host = "[::1]:8080"

	if authority := Authority(request, false); authority != "[::1]:8080" {
		t.Fatalf("got %q, want %q", authority, "[::1]:8080")
	}
}

// A port is digits. Anything else after the host's colon would pass a comparison
// made on the host and be repeated wherever the authority is, so it yields no
// host at all.
func TestHostFromAuthority(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		authority string
		expected  string
	}{
		{name: "a host", authority: "open.home.arpa", expected: "open.home.arpa"},
		{name: "a host and a port", authority: "open.home.arpa:443", expected: "open.home.arpa"},
		{name: "an empty port", authority: "open.home.arpa:", expected: "open.home.arpa"},
		{name: "a port with leading zeros", authority: "open.home.arpa:0443", expected: "open.home.arpa"},
		{name: "a bracketed address and a port", authority: "[::1]:8080", expected: "::1"},
		{name: "a port followed by forwarded parameters", authority: "open.home.arpa:443,for=127.0.0.1;by=x", expected: ""},
		{name: "a port followed by a semicolon", authority: "open.home.arpa:443;x", expected: ""},
		{name: "a port that is a word", authority: "open.home.arpa:https", expected: ""},
		{name: "a port with a sign", authority: "open.home.arpa:+443", expected: ""},
		{name: "a port with a space", authority: "open.home.arpa:443 x", expected: ""},
		{name: "a bracketed address and a port that is a word", authority: "[::1]:x", expected: ""},
		{name: "a bracketed address", authority: "[2001:DB8::1]", expected: "2001:db8::1"},
		{name: "a bracketed name and a port", authority: "[open.home.arpa]:443", expected: ""},
		{name: "a bracketed name", authority: "[open.home.arpa]", expected: ""},
		{name: "a bracketed IPv4 address", authority: "[127.0.0.1]:443", expected: ""},
		{name: "an unclosed bracket", authority: "[open.home.arpa", expected: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if host := HostFromAuthority(testCase.authority); host != testCase.expected {
				t.Fatalf("HostFromAuthority(%q) = %q, want %q", testCase.authority, host, testCase.expected)
			}
		})
	}
}

// firebaseEdges is the range of Firebase Hosting's edges as Google's front end
// recorded them.
var firebaseEdges = netip.MustParsePrefix("66.249.93.0/24")

// The client's address is believed only from the headers a deployment says a
// proxy in front writes: X-Client-Ip-Address as the load balancer sent it, and
// X-Forwarded-For read from the right, past the trusted proxies.
func TestClientAddress(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                      string
		lines                     map[string][]string
		trustForwardedFor         bool
		trustClientHeaders        bool
		trustedProxies            []netip.Prefix
		expected                  string
		expectedFromClientHeaders bool
	}{
		{
			name:     "nothing trusted",
			lines:    map[string][]string{"X-Forwarded-For": {"192.0.2.1"}, "X-Client-Ip-Address": {"192.0.2.3"}},
			expected: "",
		},
		{
			name:              "run.app directly, the front end appending the caller",
			lines:             map[string][]string{"X-Forwarded-For": {"192.0.2.21,203.0.113.9"}},
			trustForwardedFor: true,
			expected:          "203.0.113.9",
		},
		{
			name:              "run.app directly, with the edges trusted",
			lines:             map[string][]string{"X-Forwarded-For": {"192.0.2.21,203.0.113.9"}},
			trustForwardedFor: true,
			trustedProxies:    []netip.Prefix{firebaseEdges},
			expected:          "203.0.113.9",
		},
		{
			name:              "through Firebase Hosting, the edge trusted",
			lines:             map[string][]string{"X-Forwarded-For": {"198.51.100.7,66.249.93.77"}},
			trustForwardedFor: true,
			trustedProxies:    []netip.Prefix{firebaseEdges},
			expected:          "198.51.100.7",
		},
		{
			name:              "through Firebase Hosting, the edge not trusted",
			lines:             map[string][]string{"X-Forwarded-For": {"198.51.100.7,66.249.93.77"}},
			trustForwardedFor: true,
			expected:          "66.249.93.77",
		},
		{
			name:              "a chain naming an edge, sent directly",
			lines:             map[string][]string{"X-Forwarded-For": {"198.51.100.7,66.249.93.77,203.0.113.9"}},
			trustForwardedFor: true,
			trustedProxies:    []netip.Prefix{firebaseEdges},
			expected:          "203.0.113.9",
		},
		{
			name:              "reverse_proxy's single entry",
			lines:             map[string][]string{"X-Forwarded-For": {"198.51.100.7"}},
			trustForwardedFor: true,
			expected:          "198.51.100.7",
		},
		{
			name:              "two trusted proxies",
			lines:             map[string][]string{"X-Forwarded-For": {"192.0.2.21, 198.51.100.7, 66.249.93.133, 66.249.93.77"}},
			trustForwardedFor: true,
			trustedProxies:    []netip.Prefix{firebaseEdges},
			expected:          "198.51.100.7",
		},
		{
			name:              "every entry a trusted proxy's",
			lines:             map[string][]string{"X-Forwarded-For": {"66.249.93.133, 66.249.93.77"}},
			trustForwardedFor: true,
			trustedProxies:    []netip.Prefix{firebaseEdges},
			expected:          "",
		},
		{
			name:              "an empty entry reached is not skipped",
			lines:             map[string][]string{"X-Forwarded-For": {"198.51.100.7, ,66.249.93.77"}},
			trustForwardedFor: true,
			trustedProxies:    []netip.Prefix{firebaseEdges},
			expected:          "",
		},
		{
			name:              "a garbage entry reached is not skipped",
			lines:             map[string][]string{"X-Forwarded-For": {"198.51.100.7,unknown,66.249.93.77"}},
			trustForwardedFor: true,
			trustedProxies:    []netip.Prefix{firebaseEdges},
			expected:          "",
		},
		{
			name:              "an empty rightmost entry",
			lines:             map[string][]string{"X-Forwarded-For": {"198.51.100.7,"}},
			trustForwardedFor: true,
			expected:          "",
		},
		{
			name:              "garbage beyond the client is never reached",
			lines:             map[string][]string{"X-Forwarded-For": {"unknown, ,198.51.100.7,66.249.93.77"}},
			trustForwardedFor: true,
			trustedProxies:    []netip.Prefix{firebaseEdges},
			expected:          "198.51.100.7",
		},
		{
			name:              "as many trusted proxies as the walk steps over",
			lines:             map[string][]string{"X-Forwarded-For": {"198.51.100.7" + strings.Repeat(",66.249.93.77", maxProxyHops)}},
			trustForwardedFor: true,
			trustedProxies:    []netip.Prefix{firebaseEdges},
			expected:          "198.51.100.7",
		},
		{
			name:              "more trusted proxies than the walk steps over",
			lines:             map[string][]string{"X-Forwarded-For": {"198.51.100.7" + strings.Repeat(",66.249.93.77", maxProxyHops+1)}},
			trustForwardedFor: true,
			trustedProxies:    []netip.Prefix{firebaseEdges},
			expected:          "",
		},
		{
			name:              "two lines, the last line's entry is the rightmost",
			lines:             map[string][]string{"X-Forwarded-For": {"192.0.2.21", "203.0.113.9"}},
			trustForwardedFor: true,
			expected:          "203.0.113.9",
		},
		{
			name:              "two lines, read past a trusted proxy into the first",
			lines:             map[string][]string{"X-Forwarded-For": {"192.0.2.21, 198.51.100.7", "66.249.93.77"}},
			trustForwardedFor: true,
			trustedProxies:    []netip.Prefix{firebaseEdges},
			expected:          "198.51.100.7",
		},
		{
			name:              "two lines, the last one empty",
			lines:             map[string][]string{"X-Forwarded-For": {"198.51.100.7", ""}},
			trustForwardedFor: true,
			expected:          "",
		},
		{
			name:              "spaces around the entries",
			lines:             map[string][]string{"X-Forwarded-For": {"  198.51.100.7  ,  66.249.93.77  "}},
			trustForwardedFor: true,
			trustedProxies:    []netip.Prefix{firebaseEdges},
			expected:          "198.51.100.7",
		},
		{
			name:              "an IPv4 address and a port",
			lines:             map[string][]string{"X-Forwarded-For": {"192.0.2.1:54321"}},
			trustForwardedFor: true,
			expected:          "192.0.2.1",
		},
		{
			name:              "a bare IPv6 address",
			lines:             map[string][]string{"X-Forwarded-For": {"192.0.2.21, 2001:DB8::1"}},
			trustForwardedFor: true,
			expected:          "2001:db8::1",
		},
		{
			name:              "a bracketed IPv6 address",
			lines:             map[string][]string{"X-Forwarded-For": {"[2001:db8::1]"}},
			trustForwardedFor: true,
			expected:          "2001:db8::1",
		},
		{
			name:              "a bracketed IPv6 address and a port",
			lines:             map[string][]string{"X-Forwarded-For": {"[2001:db8::1]:443"}},
			trustForwardedFor: true,
			expected:          "2001:db8::1",
		},
		{
			name:              "an IPv4-mapped IPv6 address",
			lines:             map[string][]string{"X-Forwarded-For": {"::ffff:192.0.2.1"}},
			trustForwardedFor: true,
			expected:          "192.0.2.1",
		},
		{
			name:              "an IPv4-mapped trusted proxy",
			lines:             map[string][]string{"X-Forwarded-For": {"198.51.100.7, [::ffff:66.249.93.77]:443"}},
			trustForwardedFor: true,
			trustedProxies:    []netip.Prefix{firebaseEdges},
			expected:          "198.51.100.7",
		},
		{
			name:              "an IPv6 trusted proxy",
			lines:             map[string][]string{"X-Forwarded-For": {"2001:db8::5, 2001:db8:f::1"}},
			trustForwardedFor: true,
			trustedProxies:    []netip.Prefix{netip.MustParsePrefix("2001:db8:f::/48")},
			expected:          "2001:db8::5",
		},
		{
			name:              "an address with a zone",
			lines:             map[string][]string{"X-Forwarded-For": {"198.51.100.1, fe80::1%eth0"}},
			trustForwardedFor: true,
			expected:          "",
		},
		{
			name:              "a bracketed address with a zone and a port",
			lines:             map[string][]string{"X-Forwarded-For": {"[fe80::1%eth0]:443"}},
			trustForwardedFor: true,
			expected:          "",
		},
		{
			name:              "a bracketed IPv4 address",
			lines:             map[string][]string{"X-Forwarded-For": {"[192.0.2.1]"}},
			trustForwardedFor: true,
			expected:          "",
		},
		{
			name:              "a name",
			lines:             map[string][]string{"X-Forwarded-For": {"198.51.100.1, client.example.com"}},
			trustForwardedFor: true,
			expected:          "",
		},
		{
			name:              "no header",
			trustForwardedFor: true,
			expected:          "",
		},
		{
			name:                      "the client headers, trusted",
			lines:                     map[string][]string{"X-Client-Ip-Address": {"203.0.113.5"}, "X-Forwarded-For": {"192.0.2.1"}},
			trustForwardedFor:         true,
			trustClientHeaders:        true,
			expected:                  "203.0.113.5",
			expectedFromClientHeaders: true,
		},
		{
			name:                      "the client address as sent, mapped",
			lines:                     map[string][]string{"X-Client-Ip-Address": {"::ffff:203.0.113.5"}},
			trustClientHeaders:        true,
			expected:                  "::ffff:203.0.113.5",
			expectedFromClientHeaders: true,
		},
		{
			name:                      "the client address as sent, trimmed and not folded",
			lines:                     map[string][]string{"X-Client-Ip-Address": {" 2001:DB8::3 "}},
			trustClientHeaders:        true,
			expected:                  "2001:DB8::3",
			expectedFromClientHeaders: true,
		},
		{
			name:               "a client address with a port is not an IP address",
			lines:              map[string][]string{"X-Client-Ip-Address": {"203.0.113.5:443"}},
			trustClientHeaders: true,
			expected:           "",
		},
		{
			name:               "a client address with a zone",
			lines:              map[string][]string{"X-Client-Ip-Address": {"fe80::1%eth0"}},
			trustClientHeaders: true,
			expected:           "",
		},
		{
			name:               "the client headers not trusted, X-Forwarded-For trusted",
			lines:              map[string][]string{"X-Client-Ip-Address": {"203.0.113.5"}, "X-Forwarded-For": {"192.0.2.1"}},
			trustForwardedFor:  true,
			trustClientHeaders: false,
			expected:           "192.0.2.1",
		},
		{
			name:               "an invalid client address falls to a trusted X-Forwarded-For",
			lines:              map[string][]string{"X-Client-Ip-Address": {"not an address"}, "X-Forwarded-For": {"192.0.2.1"}},
			trustForwardedFor:  true,
			trustClientHeaders: true,
			expected:           "192.0.2.1",
		},
		{
			name:               "an invalid client address and an untrusted X-Forwarded-For",
			lines:              map[string][]string{"X-Client-Ip-Address": {"not an address"}, "X-Forwarded-For": {"192.0.2.1"}},
			trustClientHeaders: true,
			expected:           "",
		},
		{
			name:               "a client address given on two lines",
			lines:              map[string][]string{"X-Client-Ip-Address": {"203.0.113.5", "192.0.2.4"}},
			trustClientHeaders: true,
			expected:           "",
		},
		{
			name:               "a client address given on two lines falls to a trusted X-Forwarded-For",
			lines:              map[string][]string{"X-Client-Ip-Address": {"203.0.113.5", "192.0.2.4"}, "X-Forwarded-For": {"192.0.2.1"}},
			trustForwardedFor:  true,
			trustClientHeaders: true,
			expected:           "192.0.2.1",
		},
		{
			name:               "Forwarded alone, everything trusted",
			lines:              map[string][]string{"Forwarded": {"for=192.0.2.2;proto=https"}},
			trustForwardedFor:  true,
			trustClientHeaders: true,
			trustedProxies:     []netip.Prefix{firebaseEdges},
			expected:           "",
		},
		{
			name: "Forwarded ahead of X-Forwarded-For",
			lines: map[string][]string{
				"Forwarded":       {"for=192.0.2.2"},
				"X-Forwarded-For": {"192.0.2.1"},
			},
			trustForwardedFor: true,
			expected:          "192.0.2.1",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			header := http.Header{}
			for name, lines := range testCase.lines {
				for _, line := range lines {
					header.Add(name, line)
				}
			}

			address, fromClientHeaders := ClientAddress(
				header,
				testCase.trustForwardedFor,
				testCase.trustClientHeaders,
				testCase.trustedProxies,
			)
			if address != testCase.expected || fromClientHeaders != testCase.expectedFromClientHeaders {
				t.Fatalf(
					"ClientAddress(%v, %t, %t, %v) = %q, %t; want %q, %t",
					testCase.lines,
					testCase.trustForwardedFor,
					testCase.trustClientHeaders,
					testCase.trustedProxies,
					address,
					fromClientHeaders,
					testCase.expected,
					testCase.expectedFromClientHeaders,
				)
			}
		})
	}
}

// The forwarded host is believed where it is trusted and, where trusted
// proxies are known, only of a request the proxy directly in front recorded as
// coming from one.
func TestForwardedHostTrusted(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		lines              []string
		trustForwardedHost bool
		trustedProxies     []netip.Prefix
		expected           bool
	}{
		{
			name:           "not trusted, through Firebase Hosting",
			lines:          []string{"198.51.100.7,66.249.93.77"},
			trustedProxies: []netip.Prefix{firebaseEdges},
			expected:       false,
		},
		{
			name:               "trusted with no proxies, run.app directly",
			lines:              []string{"192.0.2.21,203.0.113.9"},
			trustForwardedHost: true,
			expected:           true,
		},
		{
			name:               "trusted with no proxies and no X-Forwarded-For",
			trustForwardedHost: true,
			expected:           true,
		},
		{
			name:               "through Firebase Hosting",
			lines:              []string{"198.51.100.7,66.249.93.77"},
			trustForwardedHost: true,
			trustedProxies:     []netip.Prefix{firebaseEdges},
			expected:           true,
		},
		{
			name:               "run.app directly",
			lines:              []string{"192.0.2.21,203.0.113.9"},
			trustForwardedHost: true,
			trustedProxies:     []netip.Prefix{firebaseEdges},
			expected:           false,
		},
		{
			name:               "a chain naming an edge, sent directly",
			lines:              []string{"198.51.100.7,66.249.93.77,203.0.113.9"},
			trustForwardedHost: true,
			trustedProxies:     []netip.Prefix{firebaseEdges},
			expected:           false,
		},
		{
			name:               "no X-Forwarded-For",
			trustForwardedHost: true,
			trustedProxies:     []netip.Prefix{firebaseEdges},
			expected:           false,
		},
		{
			name:               "the edge on a line of its own",
			lines:              []string{"198.51.100.7", "66.249.93.77"},
			trustForwardedHost: true,
			trustedProxies:     []netip.Prefix{firebaseEdges},
			expected:           true,
		},
		{
			name:               "an edge on the first line only",
			lines:              []string{"66.249.93.77", "203.0.113.9"},
			trustForwardedHost: true,
			trustedProxies:     []netip.Prefix{firebaseEdges},
			expected:           false,
		},
		{
			name:               "an empty rightmost entry",
			lines:              []string{"66.249.93.77,"},
			trustForwardedHost: true,
			trustedProxies:     []netip.Prefix{firebaseEdges},
			expected:           false,
		},
		{
			name:               "a rightmost entry that is not an address",
			lines:              []string{"66.249.93.77, unknown"},
			trustForwardedHost: true,
			trustedProxies:     []netip.Prefix{firebaseEdges},
			expected:           false,
		},
		{
			name:               "an edge with a port",
			lines:              []string{"198.51.100.7, 66.249.93.77:443"},
			trustForwardedHost: true,
			trustedProxies:     []netip.Prefix{firebaseEdges},
			expected:           true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			header := http.Header{}
			for _, line := range testCase.lines {
				header.Add("X-Forwarded-For", line)
			}

			trusted := ForwardedHostTrusted(header, testCase.trustForwardedHost, testCase.trustedProxies)
			if trusted != testCase.expected {
				t.Fatalf(
					"ForwardedHostTrusted(%q, %t, %v) = %t, want %t",
					testCase.lines,
					testCase.trustForwardedHost,
					testCase.trustedProxies,
					trusted,
					testCase.expected,
				)
			}
		})
	}
}

// A client header is read only where it is on exactly one line.
func TestClientHeader(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		lines    []string
		expected string
	}{
		{name: "absent", expected: ""},
		{name: "one line", lines: []string{"Stockholm"}, expected: "Stockholm"},
		{name: "one empty line", lines: []string{""}, expected: ""},
		{name: "two lines", lines: []string{"Stockholm", "Oslo"}, expected: ""},
		{name: "two lines, one empty", lines: []string{"", "Stockholm"}, expected: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			header := http.Header{}
			for _, line := range testCase.lines {
				header.Add("X-Client-Geo-City-Name", line)
			}

			if value := ClientHeader(header, "X-Client-Geo-City-Name"); value != testCase.expected {
				t.Fatalf("ClientHeader(%q) = %q, want %q", testCase.lines, value, testCase.expected)
			}
		})
	}
}
