package mux

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
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
// outer one resolved wherever the outer one trusted anything, resolving the
// client itself only where the outer one trusted nothing.
func TestMux_ResolvesClient(t *testing.T) {
	t.Parallel()

	const servedHost = "open.home.arpa"

	type trust struct {
		forwardedFor   bool
		clientHeaders  bool
		trustedProxies []netip.Prefix
	}

	// The proxy directly in front appended 203.0.113.9, which a proxy further out
	// in 203.0.113.0/24 would have named 192.0.2.21 as having connected to it.
	defaultLines := map[string][]string{
		"X-Forwarded-For":     {"192.0.2.21, 203.0.113.9"},
		"X-Client-Ip-Address": {"192.0.2.3"},
		"Forwarded":           {"for=192.0.2.2"},
	}
	everyAddress := []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")}
	outerRange := []netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")}

	testCases := []struct {
		name                      string
		outer                     *trust
		inner                     *trust
		lines                     map[string][]string
		expectedOuterAddress      string
		expectedOuterClientHeader bool
		expectedInnerAddress      string
		expectedInnerClientHeader bool
	}{
		{
			name:  "nothing trusted",
			outer: &trust{},
			inner: &trust{},
		},
		{
			name:                 "the inner mux inherits a trusted X-Forwarded-For",
			outer:                &trust{forwardedFor: true},
			inner:                &trust{},
			expectedOuterAddress: "203.0.113.9",
			expectedInnerAddress: "203.0.113.9",
		},
		{
			name:                      "the inner mux inherits trusted client headers",
			outer:                     &trust{clientHeaders: true},
			inner:                     &trust{},
			expectedOuterAddress:      "192.0.2.3",
			expectedOuterClientHeader: true,
			expectedInnerAddress:      "192.0.2.3",
			expectedInnerClientHeader: true,
		},
		{
			name:                 "an inner mux resolves for itself where the outer one trusts nothing",
			outer:                &trust{},
			inner:                &trust{forwardedFor: true, trustedProxies: outerRange},
			expectedInnerAddress: "192.0.2.21",
		},
		{
			name:                      "an inner mux with settings of its own inherits all the same",
			outer:                     &trust{clientHeaders: true},
			inner:                     &trust{forwardedFor: true},
			expectedOuterAddress:      "192.0.2.3",
			expectedOuterClientHeader: true,
			expectedInnerAddress:      "192.0.2.3",
			expectedInnerClientHeader: true,
		},
		{
			name:                 "an inner mux with other trusted proxies inherits all the same",
			outer:                &trust{forwardedFor: true},
			inner:                &trust{forwardedFor: true, trustedProxies: outerRange},
			expectedOuterAddress: "203.0.113.9",
			expectedInnerAddress: "203.0.113.9",
		},
		{
			name:  "an outer mux that trusted something and resolved nothing is inherited",
			outer: &trust{forwardedFor: true, trustedProxies: everyAddress},
			inner: &trust{clientHeaders: true},
		},
		{
			name:                 "trusted client headers that name nothing leave them untrusted",
			outer:                &trust{forwardedFor: true, clientHeaders: true},
			inner:                &trust{},
			lines:                map[string][]string{"X-Forwarded-For": {"192.0.2.21, 203.0.113.9"}, "X-Client-Ip-Address": {"192.0.2.3", "192.0.2.4"}},
			expectedOuterAddress: "203.0.113.9",
			expectedInnerAddress: "203.0.113.9",
		},
		{
			name:                      "a mux with no outer one",
			inner:                     &trust{forwardedFor: true, clientHeaders: true},
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
			if innerTrust := testCase.inner; innerTrust != nil {
				inner.TrustForwardedFor = innerTrust.forwardedFor
				inner.TrustClientHeaders = innerTrust.clientHeaders
				inner.TrustedProxies = innerTrust.trustedProxies
			}

			var innerHttpContext, outerHttpContext *altshiftHttpTypes.HttpContext
			inner.DoneCallback = capturedHttpContext(&innerHttpContext)

			var handler http.Handler = inner
			if outerTrust := testCase.outer; outerTrust != nil {
				vhostMux := &VhostMux{
					HostToSpecification: map[string]*VhostMuxSpecification{servedHost: {Mux: inner}},
				}
				vhostMux.TrustForwardedFor = outerTrust.forwardedFor
				vhostMux.TrustClientHeaders = outerTrust.clientHeaders
				vhostMux.TrustedProxies = outerTrust.trustedProxies
				vhostMux.DoneCallback = capturedHttpContext(&outerHttpContext)
				handler = vhostMux
			}

			lines := testCase.lines
			if lines == nil {
				lines = defaultLines
			}

			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+servedHost+"/", nil)
			request.RemoteAddr = "198.51.100.200:54321"
			for name, values := range lines {
				for _, value := range values {
					request.Header.Add(name, value)
				}
			}

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
				if outerHttpContext.TrustClientHeaders != testCase.expectedOuterClientHeader {
					t.Errorf("outer trusts client headers = %t, want %t", outerHttpContext.TrustClientHeaders, testCase.expectedOuterClientHeader)
				}
			}
		})
	}
}

// misdirectedForwardedFor is an X-Forwarded-For whose client the summary of a
// misdirected request does not keep: the client is reached from the right past
// as many trusted proxies as push it out of the end the summary keeps, and the
// entries before it push it out of the start, so the client can only have been
// resolved from the request as it arrived.
func misdirectedForwardedFor(client string, proxy string) string {
	// Ten proxies' entries are more than the half of misdirectedValueLimit the
	// summary keeps of the end, and fewer than the walk steps over.
	return strings.Repeat("192.0.2.99, ", misdirectedValueLimit/8) + client + strings.Repeat(", "+proxy, 10)
}

// loggedRecords returns each record logged, as JSON decodes it.
func loggedRecords(t *testing.T, logged string) []map[string]any {
	t.Helper()

	var records []map[string]any
	for line := range strings.Lines(logged) {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("json unmarshal: %v: %s", err, line)
		}
		records = append(records, record)
	}

	return records
}

// loggedField returns the field of a record at a dotted path, and whether there
// is one.
func loggedField(record map[string]any, path string) (string, bool) {
	var value any = record
	for key := range strings.SplitSeq(path, ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return "", false
		}
		if value, ok = object[key]; !ok {
			return "", false
		}
	}

	text, ok := value.(string)
	return text, ok
}

// A request the vhost mux answers with 421 is logged from a summary of it, which
// keeps a bounded part of X-Forwarded-For; the client it was resolved to on
// arrival is logged all the same.
//
//nolint:paralleltest // Swaps the default logger, which is global, to read what is logged.
func TestVhostMux_MisdirectedRequestKeepsResolvedClient(t *testing.T) {
	const client = "198.51.100.7"
	const proxy = "66.249.93.77"
	trustedProxies := []netip.Prefix{netip.MustParsePrefix("66.249.93.0/24")}

	vhostMux := &VhostMux{
		HostToSpecification: map[string]*VhostMuxSpecification{
			"open.home.arpa": {Mux: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("the request reached a vhost")
			})},
		},
	}
	vhostMux.TrustForwardedFor = true
	vhostMux.TrustedProxies = trustedProxies

	var httpContext *altshiftHttpTypes.HttpContext
	vhostMux.DoneCallback = capturedHttpContext(&httpContext)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://open.home.arpa/", nil)
	request.Host = "unknown.home.arpa"
	request.RemoteAddr = "203.0.113.9:54321"
	request.TLS = &tls.ConnectionState{ServerName: "unknown.home.arpa"}
	request.Header.Set("X-Forwarded-For", misdirectedForwardedFor(client, proxy))

	if address, _ := forwarded_headers.ClientAddress(request.Header, true, false, trustedProxies); address != client {
		t.Fatalf("the request as it came resolves to %q, want %q", address, client)
	}

	logged, status := captureVhostLog(t, vhostMux, request)

	if status != http.StatusMisdirectedRequest {
		t.Fatalf("status = %d, want %d", status, http.StatusMisdirectedRequest)
	}
	if httpContext == nil {
		t.Fatal("no HTTP context was logged")
	}

	summary := httpContext.Request
	if summary == nil || summary == request {
		t.Fatal("the HTTP context does not describe a summary of the request")
	}
	// What the test is about: a client resolved again from the summary is not the client.
	if address, _ := forwarded_headers.ClientAddress(summary.Header, true, false, trustedProxies); address == client {
		t.Fatalf("the summary resolves to %q too, so the test cannot tell when the client was resolved", address)
	}

	if httpContext.ClientAddress != client {
		t.Errorf("client address = %q, want %q", httpContext.ClientAddress, client)
	}

	records := loggedRecords(t, logged)
	if len(records) == 0 {
		t.Fatal("nothing was logged")
	}
	for _, record := range records {
		clientIp, _ := loggedField(record, "client.ip")
		sourceIp, _ := loggedField(record, "source.ip")
		if clientIp != client || sourceIp != "203.0.113.9" {
			t.Errorf("logged client %q and source %q, want %q and %q:\n%s", clientIp, sourceIp, client, "203.0.113.9", logged)
		}
	}
}

// A 421 behind a load balancer whose client headers are trusted keeps
// X-Client-Ip-Address in its summary, and logs the client and the client headers
// as they were read on arrival: a header on two lines is not believed there, and
// is not in the log of the summary either.
//
//nolint:paralleltest // Swaps the default logger, which is global, to read what is logged.
func TestVhostMux_MisdirectedRequestKeepsClientHeaders(t *testing.T) {
	const client = "203.0.113.5"

	vhostMux := &VhostMux{
		HostToSpecification: map[string]*VhostMuxSpecification{
			"open.home.arpa": {Mux: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("the request reached a vhost")
			})},
		},
	}
	vhostMux.TrustClientHeaders = true

	var httpContext *altshiftHttpTypes.HttpContext
	vhostMux.DoneCallback = capturedHttpContext(&httpContext)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://open.home.arpa/", nil)
	request.Host = "unknown.home.arpa"
	request.RemoteAddr = "10.0.0.1:54321"
	request.Header.Set("X-Client-Ip-Address", client)
	request.Header.Set("X-Client-Geo-Country-Iso-Code", "SE")
	request.Header.Add("X-Client-Geo-City-Name", "Stockholm")
	request.Header.Add("X-Client-Geo-City-Name", "Oslo")
	request.Header.Add("X-Client-Geo-City-Name", "Bergen")

	logged, status := captureVhostLog(t, vhostMux, request)

	if status != http.StatusMisdirectedRequest {
		t.Fatalf("status = %d, want %d", status, http.StatusMisdirectedRequest)
	}
	if httpContext == nil || httpContext.Request == nil || httpContext.Request == request {
		t.Fatal("the HTTP context does not describe a summary of the request")
	}

	summaryHeader := httpContext.Request.Header
	if got := summaryHeader.Values("X-Client-Ip-Address"); len(got) != 1 || got[0] != client {
		t.Errorf("the summary's X-Client-Ip-Address = %q, want %q", got, client)
	}
	if got := summaryHeader.Values("X-Client-Geo-City-Name"); len(got) != 2 || got[0] != "Stockholm" || got[1] != "Bergen" {
		t.Errorf("the summary's X-Client-Geo-City-Name = %q, want its first and last line", got)
	}
	if !httpContext.TrustClientHeaders || httpContext.ClientAddress != client {
		t.Errorf("resolved %q, trusting client headers %t; want %q, true", httpContext.ClientAddress, httpContext.TrustClientHeaders, client)
	}

	records := loggedRecords(t, logged)
	if len(records) == 0 {
		t.Fatal("nothing was logged")
	}
	for _, record := range records {
		if got, _ := loggedField(record, "client.ip"); got != client {
			t.Errorf("client.ip = %q, want %q:\n%s", got, client, logged)
		}
		if got, _ := loggedField(record, "client.geo.country_iso_code"); got != "SE" {
			t.Errorf("client.geo.country_iso_code = %q, want %q:\n%s", got, "SE", logged)
		}
		if got, ok := loggedField(record, "client.geo.city_name"); ok {
			t.Errorf("client.geo.city_name = %q, want it absent, the header being on more than one line:\n%s", got, logged)
		}
	}
}
