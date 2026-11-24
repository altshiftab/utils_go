package mux

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	muxErrors "github.com/altshiftab/utils_go/pkg/http/mux/errors"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/forwarded_headers"
	muxTypesResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	muxTypesResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	muxTypesResponseWriter "github.com/altshiftab/utils_go/pkg/http/mux/types/response_writer"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	altshiftStrings "github.com/altshiftab/utils_go/pkg/strings"
)

type VhostMuxSpecification struct {
	Mux         http.Handler
	RedirectTo  string
	Certificate *tls.Certificate
}

type VhostMux struct {
	baseMux
	HostToSpecification map[string]*VhostMuxSpecification
	// TrustForwardedHost makes the host looked up below the one the forwarded
	// headers name, rather than the one on the request. It is for a service
	// behind a proxy that rewrites Host to an address of its own -- Firebase
	// Hosting rewriting to a run.app URL, say -- where without it every request
	// arrives for a host this mux does not answer for and is refused.
	//
	// It is off by default, and turning it on is a decision about what can
	// reach the service rather than about what is in front of it: the headers
	// are the client's to write unless a proxy overwrites them AND nothing can
	// reach the service except through that proxy. Where the second does not
	// hold, the 421 below stops being a check and becomes routing.
	//
	// With TrustedProxies it holds again: the forwarded host is believed only
	// of a request whose rightmost X-Forwarded-For entry is inside them, the
	// proxy directly in front having recorded that it came through one, and a
	// request sent around them is looked up by its own Host. Without them it is
	// believed of every request. See forwarded_headers.ForwardedHostTrusted.
	TrustForwardedHost bool
}

func (vhostMux *VhostMux) PatchHttpServer(httpServer *http.Server) {
	if httpServer == nil {
		return
	}

	httpServer.Handler = vhostMux

	tlsConfig := httpServer.TLSConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{}
		httpServer.TLSConfig = tlsConfig
	}

	tlsConfig.GetCertificate = func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		if clientHello == nil {
			return nil, nil
		}

		hostToSpecification := vhostMux.HostToSpecification
		if hostToSpecification == nil {
			return nil, altshiftErrors.NewWithTrace(nil_error.New("host to mux specification"))
		}

		specification, ok := hostToSpecification[clientHello.ServerName]
		if !ok || specification == nil {
			return nil, nil
		}

		return specification.Certificate, nil
	}
}

// misdirectedValueLimit bounds each value kept of a misdirected request for the
// log. A name, an address or a user agent fits; padding does not.
const misdirectedValueLimit = 256

// misdirectedHeaders are the headers kept of a misdirected request: what the
// client says it is, what it came through, and what a load balancer in front
// adds that the log extractors read -- the trace the entry is grouped under,
// the client's address, location, ports and TLS fingerprints.
var misdirectedHeaders = []string{
	"User-Agent",
	"X-Forwarded-For",
	"Forwarded",
	"X-Forwarded-Host",
	"X-Forwarded-Proto",
	"X-Cloud-Trace-Context",
	"Traceparent",
	"X-Client-Ip-Address",
	"X-Client-Geo-City-Name",
	"X-Client-Geo-Country-Iso-Code",
	"X-Client-Geo-Region-Iso-Code",
	"X-Client-Port",
	"X-Server-Port",
	"X-Tls-Ja3-Fingerprint",
	"X-Tls-Ja4-Fingerprint",
}

// misdirectedListHeaders are the headers each proxy appends its entry to: the
// start is what the client wrote, and the end what the nearest proxy saw, so
// both are kept when one is bounded.
var misdirectedListHeaders = map[string]bool{"X-Forwarded-For": true, "Forwarded": true}

// truncateEnds bounds value as truncateValue does, but keeps its end as well as
// its start.
func truncateEnds(value string, limit int) string {
	if len(value) <= limit {
		return value
	}

	head := limit / 2
	for head > 0 && !utf8.RuneStart(value[head]) {
		head--
	}
	tail := len(value) - limit/2
	for tail < len(value) && !utf8.RuneStart(value[tail]) {
		tail++
	}

	return fmt.Sprintf("%s... [truncated, %d bytes in full] ...%s", value[:head], len(value), value[tail:])
}

// truncateValue cuts a value to at most limit bytes, at the start of a
// character, and says that it was cut.
func truncateValue(value string, limit int) string {
	if len(value) <= limit {
		return value
	}

	cut := limit
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}

	return fmt.Sprintf("%s... [truncated, %d bytes in full]", value[:cut], len(value))
}

// summarizeMisdirected is what is logged of a request no vhost answers for, in
// place of the request itself: its method, path, host and peer, the few headers
// in misdirectedHeaders, each value bounded. Such a request is anyone's to send
// before anything is asked of them, and the size of the request line and the
// headers is theirs to choose -- a megabyte of headers by net/http's default --
// so logging it in full writes as much as was sent, and more, for every request
// on a connection kept alive to send them. The query is left out: it is
// unbounded, and was meant for a vhost that may declare what in it is secret.
func summarizeMisdirected(request *http.Request) *http.Request {
	summary := request.WithContext(request.Context())

	summary.Method = truncateValue(request.Method, misdirectedValueLimit)
	summary.Host = truncateValue(request.Host, misdirectedValueLimit)
	summary.RequestURI = ""

	summaryUrl := &url.URL{}
	if requestUrl := request.URL; requestUrl != nil {
		summaryUrl.Path = truncateValue(requestUrl.Path, misdirectedValueLimit)
	}
	summary.URL = summaryUrl

	header := make(http.Header, len(misdirectedHeaders))
	for _, name := range misdirectedHeaders {
		lines := request.Header.Values(name)

		// Every line of a list counts, as in the full log: a proxy may add a
		// line of its own after the client's.
		if misdirectedListHeaders[name] {
			var values []string
			for _, value := range lines {
				if value != "" {
					values = append(values, value)
				}
			}
			if len(values) == 0 {
				continue
			}

			header.Set(name, truncateEnds(strings.Join(values, ", "), misdirectedValueLimit))
			continue
		}

		// Any other header keeps its first line and, where there are more, its
		// last, each a line of its own: the line a proxy added after the client's
		// is kept, and a header given on more than one line still reads as one,
		// which a client header is not believed from (see
		// forwarded_headers.ClientHeader). The entry is logged from the summary
		// as the request was read on arrival.
		if len(lines) > 2 {
			lines = []string{lines[0], lines[len(lines)-1]}
		}
		if !slices.ContainsFunc(lines, func(value string) bool { return value != "" }) {
			continue
		}
		for _, value := range lines {
			header.Add(name, truncateValue(value, misdirectedValueLimit))
		}
	}
	summary.Header = header
	summary.Trailer = nil
	summary.Form = nil
	summary.PostForm = nil
	summary.MultipartForm = nil

	return summary
}

// misdirected answers a request no vhost answers for with a 421, and has the
// HTTP context the mux logs it from describe a summary of it rather than the
// request as it came; see summarizeMisdirected.
func misdirected(request *http.Request, err error) *muxTypesResponseError.ResponseError {
	if httpContext, ok := request.Context().Value(MuxHttpContextContextKey).(*altshiftHttpTypes.HttpContext); ok && httpContext != nil {
		httpContext.Request = summarizeMisdirected(request)
	}

	return &muxTypesResponseError.ResponseError{
		ProblemDetail: problem_detail.New(http.StatusMisdirectedRequest),
		ClientError:   err,
	}
}

func vhostMuxHandleRequest(
	vhostMux *VhostMux,
	request *http.Request,
	responseWriter http.ResponseWriter,
) (*muxTypesResponse.Response, *muxTypesResponseError.ResponseError) {
	if vhostMux == nil {
		return nil, &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("vhost mux")),
		}
	}

	if request == nil {
		return nil, &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("request")),
		}
	}

	trustForwardedHost := forwarded_headers.ForwardedHostTrusted(
		request.Header,
		vhostMux.TrustForwardedHost,
		vhostMux.TrustedProxies,
	)
	authority := forwarded_headers.Authority(request, trustForwardedHost)
	host := forwarded_headers.HostFromAuthority(authority)

	// The TLS configuration a connection got was chosen by its server name and
	// the vhost a request goes to is chosen by its host, and nothing in HTTP
	// makes the two agree: a client can handshake as a vhost that asks nothing
	// of it and then address one that demands a client certificate. A
	// connection that sent no server name fails this too, having matched no
	// vhost's certificate to be addressing. It cannot apply where the forwarded
	// headers are believed, since the host routed on is then deliberately not
	// the one the connection was opened for; a request they are not believed of
	// is routed on its own Host, and is held to it.
	var serverName string
	if request.TLS != nil {
		serverName = truncateValue(request.TLS.ServerName, misdirectedValueLimit)
	}

	if request.TLS != nil && !trustForwardedHost {
		if forwarded_headers.HostFromAuthority(request.TLS.ServerName) != host {
			return nil, misdirected(
				request,
				fmt.Errorf(
					"%w: the server name %q is not the host %q",
					muxErrors.ErrMisdirectedRequest,
					serverName,
					truncateValue(request.Host, misdirectedValueLimit),
				),
			)
		}
	}

	hostToSpecification := vhostMux.HostToSpecification
	if hostToSpecification == nil {
		return nil, &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("host to mux specification")),
		}
	}

	muxSpecification, ok := hostToSpecification[host]
	if !ok {
		return nil, misdirected(
			request,
			fmt.Errorf(
				"%w: no vhost answers for the host %q (server name %q)",
				muxErrors.ErrMisdirectedRequest,
				truncateValue(authority, misdirectedValueLimit),
				serverName,
			),
		)
	}
	if muxSpecification == nil {
		return nil, &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("mux specification")),
		}
	}

	if redirectTo := muxSpecification.RedirectTo; redirectTo != "" {
		return &muxTypesResponse.Response{
			StatusCode: http.StatusMovedPermanently,
			Headers: []*muxTypesResponse.HeaderEntry{
				{Name: "Location", Value: altshiftStrings.HexEscapeNonASCII(redirectTo + request.RequestURI)},
			},
		}, nil
	} else if muxSpecificationMux := muxSpecification.Mux; muxSpecificationMux != nil {
		// The host was decided here, so it travels with the request rather than
		// being decided again further in. A redirector building a URL back to
		// this service reads it from there and cannot disagree with the routing
		// that got the request this far.
		muxSpecificationMux.ServeHTTP(
			responseWriter,
			request.WithContext(forwarded_headers.NewContext(request.Context(), authority)),
		)
		return nil, nil
	}

	return nil, &muxTypesResponseError.ResponseError{
		ServerError: altshiftErrors.NewWithTrace(
			muxErrors.ErrUnusableMuxSpecification,
			muxSpecification,
		),
	}
}

func (vhostMux *VhostMux) ServeHTTP(responseWriter http.ResponseWriter, request *http.Request) {
	vhostMux.ServeHttpWithCallback(
		responseWriter,
		request,
		func(request *http.Request, responseWriter *muxTypesResponseWriter.ResponseWriter) (*muxTypesResponse.Response, *muxTypesResponseError.ResponseError) {
			response, responseError := vhostMuxHandleRequest(vhostMux, request, responseWriter)
			if responseError != nil {
				responseError.ProblemDetailConverter = vhostMux.ProblemDetailConverter
			}

			if responseWriter != nil {
				responseWriter.DefaultHeaders = vhostMux.DefaultHeaders
				responseWriter.DefaultDocumentHeaders = vhostMux.DefaultDocumentHeaders
			}

			return response, responseError
		},
	)
}
