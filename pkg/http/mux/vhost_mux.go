package mux

import (
	"crypto/tls"
	"net/http"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	muxErrors "github.com/altshiftab/utils_go/pkg/http/mux/errors"
	muxTypesResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	muxTypesResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/forwarded_headers"
	muxTypesResponseWriter "github.com/altshiftab/utils_go/pkg/http/mux/types/response_writer"
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

	authority := forwarded_headers.Authority(request, vhostMux.TrustForwardedHost)
	host := forwarded_headers.HostFromAuthority(authority)

	hostToSpecification := vhostMux.HostToSpecification
	if hostToSpecification == nil {
		return nil, &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("host to mux specification")),
		}
	}

	muxSpecification, ok := hostToSpecification[host]
	if !ok {
		return nil, &muxTypesResponseError.ResponseError{
			ProblemDetail: problem_detail.New(http.StatusMisdirectedRequest),
		}
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
