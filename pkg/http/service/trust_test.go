package service

import (
	"testing"

	altshiftMux "github.com/altshiftab/utils_go/pkg/http/mux"
	"github.com/altshiftab/utils_go/pkg/http/service/service_config"
)

// What a service trusts the proxies in front to say is what its options say, nothing by default,
// and both the mux and the vhost mux in front of it are told.
func TestNewTrustSettings(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                  string
		options               []service_config.Option
		expectedForwardedFor  bool
		expectedClientHeaders bool
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

			for _, mux := range []struct {
				name               string
				trustForwardedFor  bool
				trustClientHeaders bool
			}{
				{"mux", service.Mux.TrustForwardedFor, service.Mux.TrustClientHeaders},
				{"vhost mux", vhostMux.TrustForwardedFor, vhostMux.TrustClientHeaders},
			} {
				if mux.trustForwardedFor != testCase.expectedForwardedFor {
					t.Errorf("%s trusts X-Forwarded-For = %t, want %t", mux.name, mux.trustForwardedFor, testCase.expectedForwardedFor)
				}
				if mux.trustClientHeaders != testCase.expectedClientHeaders {
					t.Errorf("%s trusts client headers = %t, want %t", mux.name, mux.trustClientHeaders, testCase.expectedClientHeaders)
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
