package cors_configurator

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func newRequestWithOrigin(t *testing.T, origin string) *http.Request {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	return request
}

func TestParse(t *testing.T) {
	t.Parallel()

	t.Run("nil request is a server error", func(t *testing.T) {
		t.Parallel()
		_, responseError := (&Configurator{}).Parse(nil)
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})

	t.Run("no origin yields no config", func(t *testing.T) {
		t.Parallel()
		configurator := &Configurator{AllowedOrigins: []string{"https://example.com"}}
		config, responseError := configurator.Parse(newRequestWithOrigin(t, ""))
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if config != nil {
			t.Fatalf("expected nil config, got %#v", config)
		}
	})

	t.Run("allowed origin matches case-insensitively", func(t *testing.T) {
		t.Parallel()
		configurator := &Configurator{AllowedOrigins: []string{"https://example.com"}, Credentials: true}
		config, responseError := configurator.Parse(newRequestWithOrigin(t, "https://EXAMPLE.com"))
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if config == nil || config.Origin != "https://example.com" {
			t.Fatalf("expected the allowed origin, got %#v", config)
		}
		if !config.Credentials {
			t.Error("expected Credentials to be carried over")
		}
	})

	t.Run("disallowed origin yields no config", func(t *testing.T) {
		t.Parallel()
		configurator := &Configurator{AllowedOrigins: []string{"https://example.com"}}
		config, responseError := configurator.Parse(newRequestWithOrigin(t, "https://evil.com"))
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if config != nil {
			t.Fatalf("expected nil config, got %#v", config)
		}
	})

	t.Run("registered domain match", func(t *testing.T) {
		t.Parallel()
		configurator := &Configurator{RegisteredDomain: "example.com"}
		config, responseError := configurator.Parse(newRequestWithOrigin(t, "https://app.example.com"))
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if config == nil || config.Origin != "https://app.example.com" {
			t.Fatalf("expected a registered-domain match, got %#v", config)
		}
	})
}

// A service configured for a local run has to recognise the page it serves as
// its own origin, and a deployed one has to go on refusing every local origin.
// The two are the same rule -- an origin matches when it shares the service's
// registered domain -- and what is tested is that the local names reach it
// rather than that they are trusted.
func TestParseLoopbackOrigins(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		registeredDomain string
		origin           string
		matches          bool
	}{
		{
			name:             "a local run recognises the page it serves",
			registeredDomain: "localhost",
			origin:           "http://localhost:8080",
			matches:          true,
		},
		{
			name:             "on whatever port it is reached",
			registeredDomain: "localhost",
			origin:           "http://localhost:3000",
			matches:          true,
		},
		{
			name:             "with the port left off, as the default port is",
			registeredDomain: "localhost",
			origin:           "http://localhost",
			matches:          true,
		},
		{
			name:             "a name reserved beneath it counts as the same deployment",
			registeredDomain: "localhost",
			origin:           "http://app.localhost:8080",
			matches:          true,
		},
		{
			name:             "so does the address the name stands for",
			registeredDomain: "localhost",
			origin:           "http://127.0.0.1:8080",
			matches:          true,
		},
		{
			name:             "and its IPv6 spelling",
			registeredDomain: "localhost",
			origin:           "http://[::1]:8080",
			matches:          true,
		},
		{
			name:             "a public origin is not part of a local deployment",
			registeredDomain: "localhost",
			origin:           "https://evil.example.com",
			matches:          false,
		},
		// The one that matters: a deployment serving a real domain must never
		// take a page on somebody's machine for one of its own, or that page
		// could read its answers with the reader's credentials attached.
		{
			name:             "a deployed service refuses a local origin",
			registeredDomain: "example.com",
			origin:           "http://localhost:8080",
			matches:          false,
		},
		{
			name:             "and refuses a name beneath the reserved one",
			registeredDomain: "example.com",
			origin:           "http://app.localhost:8080",
			matches:          false,
		},
		{
			name:             "and refuses the loopback address",
			registeredDomain: "example.com",
			origin:           "http://127.0.0.1:8080",
			matches:          false,
		},
		// An origin naming no domain is simply an origin that does not match.
		// Answering it with a 400 let any client make the service refuse.
		{
			name:             "an address that is nobody's loopback is answered, not refused",
			registeredDomain: "example.com",
			origin:           "http://8.8.8.8",
			matches:          false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			configurator := &Configurator{RegisteredDomain: testCase.registeredDomain}
			config, responseError := configurator.Parse(newRequestWithOrigin(t, testCase.origin))
			if responseError != nil {
				t.Fatalf("unexpected error: %#v", responseError)
			}

			switch {
			case testCase.matches && (config == nil || config.Origin != testCase.origin):
				t.Fatalf("expected %q to match, got %#v", testCase.origin, config)
			case !testCase.matches && config != nil:
				t.Fatalf("expected %q not to match, got %#v", testCase.origin, config)
			}
		})
	}
}
