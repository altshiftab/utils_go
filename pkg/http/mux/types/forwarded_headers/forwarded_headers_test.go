package forwarded_headers

import (
	"net/http"
	"net/http/httptest"
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

// The client's address is believed only from the headers a deployment says a
// proxy in front writes, and only from the first entry of X-Forwarded-For.
func TestClientAddress(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		lines              map[string][]string
		trustForwardedFor  bool
		trustClientHeaders bool
		expected           string
	}{
		{
			name:     "nothing trusted",
			lines:    map[string][]string{"X-Forwarded-For": {"192.0.2.1"}, "X-Client-Ip-Address": {"192.0.2.3"}},
			expected: "",
		},
		{
			name:              "one entry",
			lines:             map[string][]string{"X-Forwarded-For": {"192.0.2.1"}},
			trustForwardedFor: true,
			expected:          "192.0.2.1",
		},
		{
			name:              "two entries, the first wins",
			lines:             map[string][]string{"X-Forwarded-For": {"192.0.2.1, 198.51.100.1"}},
			trustForwardedFor: true,
			expected:          "192.0.2.1",
		},
		{
			name:              "two lines, the first line's entry wins",
			lines:             map[string][]string{"X-Forwarded-For": {"192.0.2.1", "198.51.100.1"}},
			trustForwardedFor: true,
			expected:          "192.0.2.1",
		},
		{
			name:              "spaces around the entry",
			lines:             map[string][]string{"X-Forwarded-For": {"  192.0.2.1  ,198.51.100.1"}},
			trustForwardedFor: true,
			expected:          "192.0.2.1",
		},
		{
			name:              "empty list elements before the entry",
			lines:             map[string][]string{"X-Forwarded-For": {" , ,192.0.2.1"}},
			trustForwardedFor: true,
			expected:          "192.0.2.1",
		},
		{
			name:              "an IPv4 address and a port",
			lines:             map[string][]string{"X-Forwarded-For": {"192.0.2.1:54321"}},
			trustForwardedFor: true,
			expected:          "192.0.2.1",
		},
		{
			name:              "a bare IPv6 address",
			lines:             map[string][]string{"X-Forwarded-For": {"2001:DB8::1, 198.51.100.1"}},
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
			name:              "a bracketed IPv4-mapped IPv6 address and a port",
			lines:             map[string][]string{"X-Forwarded-For": {"[::ffff:192.0.2.1]:443"}},
			trustForwardedFor: true,
			expected:          "192.0.2.1",
		},
		{
			name:              "an address with a zone",
			lines:             map[string][]string{"X-Forwarded-For": {"fe80::1%eth0, 198.51.100.1"}},
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
			name:              "unknown first, and no later entry answers",
			lines:             map[string][]string{"X-Forwarded-For": {"unknown, 198.51.100.1"}},
			trustForwardedFor: true,
			expected:          "",
		},
		{
			name:              "garbage first, and no later entry answers",
			lines:             map[string][]string{"X-Forwarded-For": {"not an address", "198.51.100.1"}},
			trustForwardedFor: true,
			expected:          "",
		},
		{
			name:              "a name first",
			lines:             map[string][]string{"X-Forwarded-For": {"client.example.com, 198.51.100.1"}},
			trustForwardedFor: true,
			expected:          "",
		},
		{
			name:              "no header",
			trustForwardedFor: true,
			expected:          "",
		},
		{
			name:               "the client headers, trusted",
			lines:              map[string][]string{"X-Client-Ip-Address": {"192.0.2.3"}, "X-Forwarded-For": {"192.0.2.1"}},
			trustForwardedFor:  true,
			trustClientHeaders: true,
			expected:           "192.0.2.3",
		},
		{
			name:               "the client headers, trusted without X-Forwarded-For",
			lines:              map[string][]string{"X-Client-Ip-Address": {" 2001:db8::3 "}, "X-Forwarded-For": {"192.0.2.1"}},
			trustClientHeaders: true,
			expected:           "2001:db8::3",
		},
		{
			name:               "the client headers not trusted, X-Forwarded-For trusted",
			lines:              map[string][]string{"X-Client-Ip-Address": {"192.0.2.3"}, "X-Forwarded-For": {"192.0.2.1"}},
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
			lines:              map[string][]string{"X-Client-Ip-Address": {"192.0.2.3", "192.0.2.4"}},
			trustClientHeaders: true,
			expected:           "",
		},
		{
			name:               "Forwarded alone, everything trusted",
			lines:              map[string][]string{"Forwarded": {"for=192.0.2.2;proto=https"}},
			trustForwardedFor:  true,
			trustClientHeaders: true,
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

			address := ClientAddress(header, testCase.trustForwardedFor, testCase.trustClientHeaders)
			if address != testCase.expected {
				t.Fatalf(
					"ClientAddress(%v, %t, %t) = %q, want %q",
					testCase.lines,
					testCase.trustForwardedFor,
					testCase.trustClientHeaders,
					address,
					testCase.expected,
				)
			}
		})
	}
}
