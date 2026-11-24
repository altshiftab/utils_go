package service

import (
	"errors"
	"os"
	"strings"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftMux "github.com/altshiftab/utils_go/pkg/http/mux"
	"github.com/altshiftab/utils_go/pkg/http/service/service_config"
)

// setOrUnsetenv sets an environment variable for the test, or unsets it where value is nil, and
// restores it when the test ends.
func setOrUnsetenv(t *testing.T, name string, value *string) {
	t.Helper()

	if value != nil {
		t.Setenv(name, *value)
		return
	}

	// Setenv records the value to restore; the variable is then unset for the test.
	t.Setenv(name, "")
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("os unsetenv: %v", err)
	}
}

// What a service trusts the proxies in front to say is an option where one is given, and the
// environment's otherwise, and both the mux and the vhost mux in front of it are told.
//
//nolint:paralleltest // Sets environment variables, which are the process's.
func TestNewTrustSettings(t *testing.T) {
	text := func(value string) *string { return &value }

	testCases := []struct {
		name                  string
		forwardedFor          *string
		clientHeaders         *string
		options               []service_config.Option
		expectedForwardedFor  bool
		expectedClientHeaders bool
		expectedErrorVariable string
	}{
		{
			name: "unset",
		},
		{
			name:          "empty",
			forwardedFor:  text(""),
			clientHeaders: text("   "),
		},
		{
			name:                 "true",
			forwardedFor:         text("true"),
			expectedForwardedFor: true,
		},
		{
			name:                  "padded 1",
			clientHeaders:         text(" 1 "),
			expectedClientHeaders: true,
		},
		{
			name:                  "both, as ParseBool spells them",
			forwardedFor:          text("TRUE"),
			clientHeaders:         text("t"),
			expectedForwardedFor:  true,
			expectedClientHeaders: true,
		},
		{
			name:          "false",
			forwardedFor:  text("false"),
			clientHeaders: text("0"),
		},
		{
			name:                  "garbage in TRUST_FORWARDED_FOR",
			forwardedFor:          text("garbage"),
			expectedErrorVariable: service_config.TrustForwardedForVariable,
		},
		{
			name:                  "garbage in TRUST_CLIENT_HEADERS",
			clientHeaders:         text("yes"),
			expectedErrorVariable: service_config.TrustClientHeadersVariable,
		},
		{
			name:          "an option overrides the environment",
			forwardedFor:  text("true"),
			clientHeaders: text("garbage"),
			options: []service_config.Option{
				service_config.WithTrustForwardedFor(false),
				service_config.WithTrustClientHeaders(true),
			},
			expectedClientHeaders: true,
		},
		{
			name: "options without the environment",
			options: []service_config.Option{
				service_config.WithTrustForwardedFor(true),
			},
			expectedForwardedFor: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			setOrUnsetenv(t, service_config.TrustForwardedForVariable, testCase.forwardedFor)
			setOrUnsetenv(t, service_config.TrustClientHeadersVariable, testCase.clientHeaders)

			options := append(
				[]service_config.Option{
					service_config.WithEndpoints(noContentEndpoint()),
					service_config.WithHost("example.com"),
				},
				testCase.options...,
			)

			service, err := New(options...)

			if testCase.expectedErrorVariable != "" {
				if err == nil {
					t.Fatal("new: got no error, want one")
				}
				if !errors.Is(err, altshiftErrors.ErrValidationError) {
					t.Errorf("new: error %v is not a validation error", err)
				}
				if !strings.Contains(err.Error(), testCase.expectedErrorVariable) {
					t.Errorf("new: error %q does not name %s", err, testCase.expectedErrorVariable)
				}
				return
			}
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
//
//nolint:paralleltest // Sets environment variables, which are the process's.
func TestNewTrustSettingsWithoutHost(t *testing.T) {
	t.Setenv(service_config.TrustClientHeadersVariable, "true")

	service, err := New(service_config.WithEndpoints(noContentEndpoint()))
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
