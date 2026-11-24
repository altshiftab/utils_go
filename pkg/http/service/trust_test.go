package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftMux "github.com/altshiftab/utils_go/pkg/http/mux"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	muxResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	muxResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/service/service_config"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

// What a service trusts the proxies in front to say is what its options say, nothing by default,
// and both the mux and the vhost mux in front of it are told.
func TestNewTrustSettings(t *testing.T) {
	t.Parallel()

	firebaseEdges := netip.MustParsePrefix("66.249.93.0/24")

	testCases := []struct {
		name                   string
		options                []service_config.Option
		expectedForwardedFor   bool
		expectedHops           int
		expectedClientHeaders  bool
		expectedTrustedProxies []netip.Prefix
	}{
		{
			name: "no option",
		},
		{
			name:                 "X-Forwarded-For",
			options:              []service_config.Option{service_config.WithTrustForwardedFor(true)},
			expectedForwardedFor: true,
		},
		{
			name:                  "the client headers",
			options:               []service_config.Option{service_config.WithTrustClientHeaders(true)},
			expectedClientHeaders: true,
		},
		{
			name: "both",
			options: []service_config.Option{
				service_config.WithTrustForwardedFor(true),
				service_config.WithTrustClientHeaders(true),
			},
			expectedForwardedFor:  true,
			expectedClientHeaders: true,
		},
		{
			name: "X-Forwarded-For past trusted proxies",
			options: []service_config.Option{
				service_config.WithTrustForwardedFor(true),
				service_config.WithTrustedProxies(firebaseEdges),
			},
			expectedForwardedFor:   true,
			expectedTrustedProxies: []netip.Prefix{firebaseEdges},
		},
		{
			name: "X-Forwarded-For two entries from the right",
			options: []service_config.Option{
				service_config.WithTrustForwardedFor(true),
				service_config.WithForwardedForHops(2),
			},
			expectedForwardedFor: true,
			expectedHops:         2,
		},
		{
			name: "turned off again",
			options: []service_config.Option{
				service_config.WithTrustForwardedFor(true),
				service_config.WithTrustForwardedFor(false),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			options := append(
				[]service_config.Option{
					service_config.WithEndpoints(noContentEndpoint()),
					service_config.WithHost("example.com"),
				},
				testCase.options...,
			)

			service, err := New(options...)
			if err != nil {
				t.Fatalf("new: %v", err)
			}

			if service.Mux == nil {
				t.Fatal("no mux")
			}
			vhostMux, ok := service.Server.Handler.(*altshiftMux.VhostMux)
			if !ok || vhostMux == nil {
				t.Fatalf("handler = %T, want a vhost mux", service.Server.Handler)
			}

			for _, mux := range []*struct {
				name               string
				trustForwardedFor  bool
				hops               int
				trustClientHeaders bool
				trustedProxies     []netip.Prefix
			}{
				{"mux", service.Mux.TrustForwardedFor, service.Mux.ForwardedForHops, service.Mux.TrustClientHeaders, service.Mux.TrustedProxies},
				{"vhost mux", vhostMux.TrustForwardedFor, vhostMux.ForwardedForHops, vhostMux.TrustClientHeaders, vhostMux.TrustedProxies},
			} {
				if mux.trustForwardedFor != testCase.expectedForwardedFor {
					t.Errorf("%s trusts X-Forwarded-For = %t, want %t", mux.name, mux.trustForwardedFor, testCase.expectedForwardedFor)
				}
				if mux.hops != testCase.expectedHops {
					t.Errorf("%s X-Forwarded-For hops = %d, want %d", mux.name, mux.hops, testCase.expectedHops)
				}
				if mux.trustClientHeaders != testCase.expectedClientHeaders {
					t.Errorf("%s trusts client headers = %t, want %t", mux.name, mux.trustClientHeaders, testCase.expectedClientHeaders)
				}
				if !slices.Equal(mux.trustedProxies, testCase.expectedTrustedProxies) {
					t.Errorf("%s trusted proxies = %v, want %v", mux.name, mux.trustedProxies, testCase.expectedTrustedProxies)
				}
			}
		})
	}
}

// A service without a host of its own serves with its mux alone, which is told what it trusts.
func TestNewTrustSettingsWithoutHost(t *testing.T) {
	t.Parallel()

	service, err := New(
		service_config.WithEndpoints(noContentEndpoint()),
		service_config.WithTrustClientHeaders(true),
	)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	mux, ok := service.Server.Handler.(*altshiftMux.Mux)
	if !ok || mux != service.Mux {
		t.Fatalf("handler = %T, want the service's mux", service.Server.Handler)
	}
	if !mux.TrustClientHeaders || mux.TrustForwardedFor {
		t.Errorf("mux trusts client headers %t and X-Forwarded-For %t, want true and false", mux.TrustClientHeaders, mux.TrustForwardedFor)
	}
}

// A service says once, as it is made, what it trusts the proxies in front of it to say, so that one
// deployed without the options says that too.
//
//nolint:paralleltest // The cases set the process-wide default logger, so they run on their own.
func TestNewLogsTrust(t *testing.T) {
	type trust struct {
		ForwardedFor       bool     `json:"forwarded_for"`
		ForwardedForHops   int      `json:"forwarded_for_hops"`
		ClientHeaders      bool     `json:"client_headers"`
		TrustedProxies     []string `json:"trusted_proxies"`
		ForwardedHost      bool     `json:"forwarded_host"`
		ForwardedHostGated bool     `json:"forwarded_host_gated"`
	}

	type loggedRecord struct {
		Message string `json:"msg"`
		Level   string `json:"level"`
		Trust   *trust `json:"trust"`
	}

	testCases := []struct {
		name     string
		options  []service_config.Option
		expected *trust
	}{
		{
			name:     "nothing trusted",
			expected: &trust{ForwardedForHops: 1, TrustedProxies: []string{}},
		},
		{
			name: "Firebase Hosting in front of Cloud Run",
			options: []service_config.Option{
				service_config.WithTrustForwardedFor(true),
				service_config.WithForwardedForHops(2),
				service_config.WithTrustForwardedHost(true),
			},
			expected: &trust{ForwardedFor: true, ForwardedForHops: 2, TrustedProxies: []string{}, ForwardedHost: true},
		},
		{
			name: "Firebase Hosting in front of Cloud Run, the edges trusted",
			options: []service_config.Option{
				service_config.WithTrustForwardedFor(true),
				service_config.WithTrustedProxies(netip.MustParsePrefix("66.249.93.0/24")),
				service_config.WithTrustForwardedHost(true),
			},
			expected: &trust{
				ForwardedFor:       true,
				ForwardedForHops:   1,
				TrustedProxies:     []string{"66.249.93.0/24"},
				ForwardedHost:      true,
				ForwardedHostGated: true,
			},
		},
		{
			name: "the forwarded host and trusted proxies, X-Forwarded-For not trusted",
			options: []service_config.Option{
				service_config.WithTrustedProxies(netip.MustParsePrefix("66.249.93.0/24")),
				service_config.WithTrustForwardedHost(true),
			},
			expected: &trust{ForwardedForHops: 1, TrustedProxies: []string{"66.249.93.0/24"}, ForwardedHost: true},
		},
		{
			name: "the forwarded host with no trusted proxies",
			options: []service_config.Option{
				service_config.WithTrustForwardedFor(true),
				service_config.WithTrustForwardedHost(true),
			},
			expected: &trust{ForwardedFor: true, ForwardedForHops: 1, TrustedProxies: []string{}, ForwardedHost: true},
		},
		{
			name:     "the client headers",
			options:  []service_config.Option{service_config.WithTrustClientHeaders(true)},
			expected: &trust{ForwardedForHops: 1, ClientHeaders: true, TrustedProxies: []string{}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var buffer bytes.Buffer
			previous := slog.Default()
			t.Cleanup(func() { slog.SetDefault(previous) })
			slog.SetDefault(slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelInfo})))

			options := append(
				[]service_config.Option{
					service_config.WithEndpoints(noContentEndpoint()),
					service_config.WithHost("example.com"),
				},
				testCase.options...,
			)
			if _, err := New(options...); err != nil {
				t.Fatalf("new: %v", err)
			}

			var records []*loggedRecord
			for line := range bytes.Lines(buffer.Bytes()) {
				var record *loggedRecord
				if err := json.Unmarshal(line, &record); err != nil {
					t.Fatalf("json unmarshal: %v: %s", err, line)
				}
				if record != nil && record.Message == trustConfiguredMessage {
					records = append(records, record)
				}
			}

			if len(records) != 1 {
				t.Fatalf("logged the trust %d times, want once:\n%s", len(records), buffer.String())
			}
			record := records[0]
			if record.Level != slog.LevelInfo.String() {
				t.Errorf("level = %q, want %q", record.Level, slog.LevelInfo.String())
			}
			if record.Trust == nil {
				t.Fatalf("no trust in the record:\n%s", buffer.String())
			}
			got, expected := record.Trust, testCase.expected
			if got.ForwardedFor != expected.ForwardedFor ||
				got.ForwardedForHops != expected.ForwardedForHops ||
				got.ClientHeaders != expected.ClientHeaders ||
				!slices.Equal(got.TrustedProxies, expected.TrustedProxies) ||
				got.ForwardedHost != expected.ForwardedHost ||
				got.ForwardedHostGated != expected.ForwardedHostGated {
				t.Errorf("trust = %+v, want %+v", *got, *expected)
			}
		})
	}
}

// The X-Forwarded-For hops are 1 unless given, 0 being taken for not given, and a service is not made
// with fewer.
func TestNewForwardedForHops(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		options         []service_config.Option
		expectedHops    int
		expectedInvalid bool
	}{
		{name: "not given"},
		{name: "0", options: []service_config.Option{service_config.WithForwardedForHops(0)}},
		{name: "1", options: []service_config.Option{service_config.WithForwardedForHops(1)}, expectedHops: 1},
		{name: "2", options: []service_config.Option{service_config.WithForwardedForHops(2)}, expectedHops: 2},
		{name: "-1", options: []service_config.Option{service_config.WithForwardedForHops(-1)}, expectedInvalid: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			options := append(
				[]service_config.Option{
					service_config.WithEndpoints(noContentEndpoint()),
					service_config.WithTrustForwardedFor(true),
				},
				testCase.options...,
			)

			service, err := New(options...)
			if testCase.expectedInvalid {
				if !errors.Is(err, altshiftErrors.ErrValidationError) {
					t.Fatalf("new: %v, want a validation error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("new: %v", err)
			}
			if service.Mux.ForwardedForHops != testCase.expectedHops {
				t.Errorf("hops = %d, want %d", service.Mux.ForwardedForHops, testCase.expectedHops)
			}
		})
	}
}

// A service made as one behind Firebase Hosting is -- the forwarded host and X-Forwarded-For trusted,
// two entries from the right, no trusted proxies -- serves a request Firebase forwarded for the host
// in X-Forwarded-Host, whatever a client's Forwarded says, and logs the client Firebase named. It
// does the same for a request sent to its run.app address directly, which is the risk it accepts.
// Made with the trusted proxies but without X-Forwarded-For trusted, it believes no forwarded host.
func TestNewBehindFirebaseHosting(t *testing.T) {
	t.Parallel()

	const servedHost = "films.vvvp.se"
	const otherHost = "evil.example"

	firebase := []service_config.Option{
		service_config.WithTrustForwardedHost(true),
		service_config.WithTrustForwardedFor(true),
		service_config.WithForwardedForHops(2),
	}

	testCases := []struct {
		name            string
		options         []service_config.Option
		headers         map[string]string
		expectedStatus  int
		expectedAddress string
	}{
		{
			name:    "through Firebase Hosting, a client's Forwarded",
			options: firebase,
			headers: map[string]string{
				"X-Forwarded-Host": servedHost,
				"Forwarded":        "host=" + otherHost,
				"X-Forwarded-For":  "198.51.100.7,66.249.93.77",
			},
			expectedStatus:  http.StatusNoContent,
			expectedAddress: "198.51.100.7",
		},
		{
			name:    "at run.app directly, the client and the host forged",
			options: firebase,
			headers: map[string]string{
				"X-Forwarded-Host": servedHost,
				"X-Forwarded-For":  "192.0.2.21,203.0.113.9",
			},
			expectedStatus:  http.StatusNoContent,
			expectedAddress: "192.0.2.21",
		},
		{
			name: "the trusted proxies without X-Forwarded-For trusted",
			options: []service_config.Option{
				service_config.WithTrustForwardedHost(true),
				service_config.WithTrustedProxies(netip.MustParsePrefix("66.249.93.0/24")),
			},
			headers: map[string]string{
				"X-Forwarded-Host": servedHost,
				"X-Forwarded-For":  "198.51.100.7,66.249.93.77",
			},
			expectedStatus: http.StatusMisdirectedRequest,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var address string
			options := append(
				[]service_config.Option{
					service_config.WithEndpoints(&endpoint.Endpoint{
						Path:   "/",
						Method: http.MethodGet,
						Public: true,
						Handler: func(request *http.Request, _ []byte) (*muxResponse.Response, *muxResponseError.ResponseError) {
							httpContext, ok := request.Context().Value(altshiftMux.MuxHttpContextContextKey).(*altshiftHttpTypes.HttpContext)
							if ok && httpContext != nil {
								address = httpContext.ClientAddress
							}
							return &muxResponse.Response{StatusCode: http.StatusNoContent}, nil
						},
					}),
					service_config.WithHost(servedHost),
				},
				testCase.options...,
			)

			service, err := New(options...)
			if err != nil {
				t.Fatalf("new: %v", err)
			}

			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://vvvp-films-abcdef.run.app/", nil)
			request.RemoteAddr = "169.254.1.1:443"
			for name, value := range testCase.headers {
				request.Header.Set(name, value)
			}

			recorder := httptest.NewRecorder()
			service.Server.Handler.ServeHTTP(recorder, request)

			if recorder.Code != testCase.expectedStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, testCase.expectedStatus)
			}
			if address != testCase.expectedAddress {
				t.Errorf("client address = %q, want %q", address, testCase.expectedAddress)
			}
		})
	}
}
