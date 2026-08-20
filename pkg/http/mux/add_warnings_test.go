package mux

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
)

// captureWarnings collects what Add says about the endpoints it is given. The
// default logger is global, so this cannot run beside another test that reads
// it -- hence no t.Parallel anywhere in this file.
func captureWarnings(t *testing.T, endpoints ...*endpointPkg.Endpoint) string {
	t.Helper()

	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	mux := &Mux{}
	mux.Add(endpoints...)

	return buffer.String()
}

func anyAuthenticationParser() request_parser.RequestParser[any] {
	return request_parser.New(func(*http.Request) (any, *response_error.ResponseError) {
		return nil, nil
	})
}

// Public and AuthenticationParser disagree in two directions, and both are
// worth hearing about: one is served to anyone, the other tells everything that
// reads Public the wrong thing about a body that is in fact gated.
//nolint:paralleltest // Reads the default logger, which is global; running beside anything else that writes to it would make the assertions depend on the order.
func TestAdd_VisibilityWarnings(t *testing.T) {
	testCases := []struct {
		name        string
		endpoint    *endpointPkg.Endpoint
		expected    string
		notExpected string
	}{
		{
			name:        "gated in name only",
			endpoint:    &endpointPkg.Endpoint{Method: http.MethodGet, Path: "/a", Public: false},
			expected:    "Non-public endpoint without authentication parser: GET /a.",
			notExpected: "Public endpoint with an authentication parser",
		},
		{
			name: "gated in fact, public in name",
			endpoint: &endpointPkg.Endpoint{
				Method:               http.MethodGet,
				Path:                 "/b",
				Public:               true,
				AuthenticationParser: anyAuthenticationParser(),
			},
			expected:    "Public endpoint with an authentication parser: GET /b.",
			notExpected: "Non-public endpoint without authentication parser",
		},
		{
			name: "agreeing: gated, and enforced",
			endpoint: &endpointPkg.Endpoint{
				Method:               http.MethodGet,
				Path:                 "/c",
				Public:               false,
				AuthenticationParser: anyAuthenticationParser(),
			},
			notExpected: "endpoint",
		},
		{
			name:        "agreeing: public, and unenforced",
			endpoint:    &endpointPkg.Endpoint{Method: http.MethodGet, Path: "/d", Public: true},
			notExpected: "endpoint",
		},
	}

	//nolint:paralleltest // Same reason: each case swaps the default logger for one it can read.
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			logged := captureWarnings(t, testCase.endpoint)

			if testCase.expected != "" && !strings.Contains(logged, testCase.expected) {
				t.Fatalf("expected a warning containing %q, got %q", testCase.expected, logged)
			}
			if testCase.notExpected != "" && strings.Contains(logged, testCase.notExpected) {
				t.Fatalf("did not expect %q in %q", testCase.notExpected, logged)
			}
		})
	}
}
