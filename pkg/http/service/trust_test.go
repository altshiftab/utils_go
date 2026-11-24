package service

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/netip"
	"slices"
	"testing"

	altshiftMux "github.com/altshiftab/utils_go/pkg/http/mux"
	"github.com/altshiftab/utils_go/pkg/http/service/service_config"
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
				trustClientHeaders bool
				trustedProxies     []netip.Prefix
			}{
				{"mux", service.Mux.TrustForwardedFor, service.Mux.TrustClientHeaders, service.Mux.TrustedProxies},
				{"vhost mux", vhostMux.TrustForwardedFor, vhostMux.TrustClientHeaders, vhostMux.TrustedProxies},
			} {
				if mux.trustForwardedFor != testCase.expectedForwardedFor {
					t.Errorf("%s trusts X-Forwarded-For = %t, want %t", mux.name, mux.trustForwardedFor, testCase.expectedForwardedFor)
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
			expected: &trust{TrustedProxies: []string{}},
		},
		{
			name: "Firebase Hosting in front of Cloud Run",
			options: []service_config.Option{
				service_config.WithTrustForwardedFor(true),
				service_config.WithTrustedProxies(netip.MustParsePrefix("66.249.93.0/24")),
				service_config.WithTrustForwardedHost(true),
			},
			expected: &trust{
				ForwardedFor:       true,
				TrustedProxies:     []string{"66.249.93.0/24"},
				ForwardedHost:      true,
				ForwardedHostGated: true,
			},
		},
		{
			name: "the forwarded host with no trusted proxies",
			options: []service_config.Option{
				service_config.WithTrustForwardedFor(true),
				service_config.WithTrustForwardedHost(true),
			},
			expected: &trust{ForwardedFor: true, TrustedProxies: []string{}, ForwardedHost: true},
		},
		{
			name:     "the client headers",
			options:  []service_config.Option{service_config.WithTrustClientHeaders(true)},
			expected: &trust{ClientHeaders: true, TrustedProxies: []string{}},
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
				got.ClientHeaders != expected.ClientHeaders ||
				!slices.Equal(got.TrustedProxies, expected.TrustedProxies) ||
				got.ForwardedHost != expected.ForwardedHost ||
				got.ForwardedHostGated != expected.ForwardedHostGated {
				t.Errorf("trust = %+v, want %+v", *got, *expected)
			}
		})
	}
}
