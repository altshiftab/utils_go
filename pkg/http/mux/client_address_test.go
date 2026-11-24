package mux

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	altshiftHttpContext "github.com/altshiftab/utils_go/pkg/http/context"
	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/forwarded_headers"
	muxResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	muxResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

// capturedHttpContext returns a done callback that keeps the HTTP context the mux
// it is set on logged the request with.
func capturedHttpContext(captured **altshiftHttpTypes.HttpContext) func(context.Context) {
	return func(ctx context.Context) {
		if httpContext, ok := ctx.Value(altshiftHttpContext.HttpContextContextKey).(*altshiftHttpTypes.HttpContext); ok {
			*captured = httpContext
		}
	}
}

// The client is resolved by the mux the request arrives at, as that mux is
// configured to believe the proxies in front, and an inner mux keeps what the
// outer one resolved unless it trusts something of its own.
func TestMux_ResolvesClient(t *testing.T) {
	t.Parallel()

	const servedHost = "open.home.arpa"

	type trust struct {
		forwardedFor  bool
		clientHeaders bool
	}

	testCases := []struct {
		name                      string
		outer                     *trust
		inner                     trust
		expectedOuterAddress      string
		expectedInnerAddress      string
		expectedInnerClientHeader bool
	}{
		{
			name:                 "nothing trusted",
			outer:                &trust{},
			expectedOuterAddress: "",
			expectedInnerAddress: "",
		},
		{
			name:                 "the inner mux inherits a trusted X-Forwarded-For",
			outer:                &trust{forwardedFor: true},
			expectedOuterAddress: "192.0.2.1",
			expectedInnerAddress: "192.0.2.1",
		},
		{
			name:                      "the inner mux inherits trusted client headers",
			outer:                     &trust{clientHeaders: true},
			expectedOuterAddress:      "192.0.2.3",
			expectedInnerAddress:      "192.0.2.3",
			expectedInnerClientHeader: true,
		},
		{
			name:                 "an inner mux that trusts X-Forwarded-For itself",
			outer:                &trust{},
			inner:                trust{forwardedFor: true},
			expectedOuterAddress: "",
			expectedInnerAddress: "192.0.2.1",
		},
		{
			name:                      "an inner mux with settings of its own is not overridden",
			outer:                     &trust{clientHeaders: true},
			inner:                     trust{forwardedFor: true},
			expectedOuterAddress:      "192.0.2.3",
			expectedInnerAddress:      "192.0.2.1",
			expectedInnerClientHeader: false,
		},
		{
			name:                      "a mux with no outer one",
			inner:                     trust{forwardedFor: true, clientHeaders: true},
			expectedInnerAddress:      "192.0.2.3",
			expectedInnerClientHeader: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			inner := New(&endpointPkg.Endpoint{
				Path:   "/",
				Method: http.MethodGet,
				Public: true,
				Handler: func(*http.Request, []byte) (*muxResponse.Response, *muxResponseError.ResponseError) {
					return &muxResponse.Response{StatusCode: http.StatusNoContent}, nil
				},
			})
			inner.TrustForwardedFor = testCase.inner.forwardedFor
			inner.TrustClientHeaders = testCase.inner.clientHeaders

			var innerHttpContext, outerHttpContext *altshiftHttpTypes.HttpContext
			inner.DoneCallback = capturedHttpContext(&innerHttpContext)

			var handler http.Handler = inner
			if testCase.outer != nil {
				vhostMux := &VhostMux{
					HostToSpecification: map[string]*VhostMuxSpecification{servedHost: {Mux: inner}},
				}
				vhostMux.TrustForwardedFor = testCase.outer.forwardedFor
				vhostMux.TrustClientHeaders = testCase.outer.clientHeaders
				vhostMux.DoneCallback = capturedHttpContext(&outerHttpContext)
				handler = vhostMux
			}

			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+servedHost+"/", nil)
			request.RemoteAddr = "203.0.113.9:54321"
			request.Header.Set("X-Forwarded-For", "192.0.2.1, 198.51.100.1")
			request.Header.Set("X-Client-Ip-Address", "192.0.2.3")
			request.Header.Set("Forwarded", "for=192.0.2.2")

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
			}

			if innerHttpContext == nil {
				t.Fatal("the inner mux logged no HTTP context")
			}
			if innerHttpContext.ClientAddress != testCase.expectedInnerAddress {
				t.Errorf("inner client address = %q, want %q", innerHttpContext.ClientAddress, testCase.expectedInnerAddress)
			}
			if innerHttpContext.TrustClientHeaders != testCase.expectedInnerClientHeader {
				t.Errorf("inner trusts client headers = %t, want %t", innerHttpContext.TrustClientHeaders, testCase.expectedInnerClientHeader)
			}

			if testCase.outer != nil {
				if outerHttpContext == nil {
					t.Fatal("the outer mux logged no HTTP context")
				}
				if outerHttpContext.ClientAddress != testCase.expectedOuterAddress {
					t.Errorf("outer client address = %q, want %q", outerHttpContext.ClientAddress, testCase.expectedOuterAddress)
				}
				if outerHttpContext.TrustClientHeaders != testCase.outer.clientHeaders {
					t.Errorf("outer trusts client headers = %t, want %t", outerHttpContext.TrustClientHeaders, testCase.outer.clientHeaders)
				}
			}
		})
	}
}

// misdirectedForwardedFor is an X-Forwarded-For whose first entry the summary
// of a misdirected request does not keep: empty list elements push it past the
// start the summary keeps, and the entries after it past the end, so the client
// can only have been resolved from the request as it arrived.
func misdirectedForwardedFor(client string) string {
	return strings.Repeat(" ,", misdirectedValueLimit/2) + client + strings.Repeat(", 198.51.100.1", misdirectedValueLimit/8)
}

// A request the vhost mux answers with 421 is logged from a summary of it, which
// keeps a bounded part of X-Forwarded-For; the client it was resolved to on
// arrival is logged all the same.
func TestVhostMux_MisdirectedRequestKeepsResolvedClient(t *testing.T) {
	t.Parallel()

	const client = "192.0.2.1"

	vhostMux := &VhostMux{
		HostToSpecification: map[string]*VhostMuxSpecification{
			"open.home.arpa": {Mux: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("the request reached a vhost")
			})},
		},
	}
	vhostMux.TrustForwardedFor = true

	var httpContext *altshiftHttpTypes.HttpContext
	vhostMux.DoneCallback = capturedHttpContext(&httpContext)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://open.home.arpa/", nil)
	request.Host = "unknown.home.arpa"
	request.RemoteAddr = "203.0.113.9:54321"
	request.TLS = &tls.ConnectionState{ServerName: "unknown.home.arpa"}
	request.Header.Set("X-Forwarded-For", misdirectedForwardedFor(client))

	if address := forwarded_headers.ClientAddress(request.Header, true, false); address != client {
		t.Fatalf("the request as it came resolves to %q, want %q", address, client)
	}

	recorder := httptest.NewRecorder()
	vhostMux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMisdirectedRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMisdirectedRequest)
	}
	if httpContext == nil {
		t.Fatal("no HTTP context was logged")
	}

	summary := httpContext.Request
	if summary == nil || summary == request {
		t.Fatal("the HTTP context does not describe a summary of the request")
	}
	// What the test is about: a client resolved again from the summary is not the client.
	if address := forwarded_headers.ClientAddress(summary.Header, true, false); address != "" {
		t.Fatalf("the summary resolves to %q, so the test cannot tell when the client was resolved", address)
	}

	if httpContext.ClientAddress != client {
		t.Errorf("client address = %q, want %q", httpContext.ClientAddress, client)
	}
}
