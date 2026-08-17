package mux

import (
	"crypto/tls"
	"net"
	"net/http"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	muxErrors "github.com/altshiftab/utils_go/pkg/http/mux/errors"
	muxTypesResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	muxTypesResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	muxTypesResponseWriter "github.com/altshiftab/utils_go/pkg/http/mux/types/response_writer"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	motmedelStrings "github.com/altshiftab/utils_go/pkg/strings"
)

type VhostMuxSpecification struct {
	Mux         http.Handler
	RedirectTo  string
	Certificate *tls.Certificate
}

type VhostMux struct {
	baseMux
	HostToSpecification map[string]*VhostMuxSpecification
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
			return nil, motmedelErrors.NewWithTrace(nil_error.New("host to mux specification"))
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
			ServerError: motmedelErrors.NewWithTrace(nil_error.New("vhost mux")),
		}
	}

	if request == nil {
		return nil, &muxTypesResponseError.ResponseError{
			ServerError: motmedelErrors.NewWithTrace(nil_error.New("request")),
		}
	}

	host, _, err := net.SplitHostPort(request.Host)
	if err != nil {
		host = request.Host
	}

	hostToSpecification := vhostMux.HostToSpecification
	if hostToSpecification == nil {
		return nil, &muxTypesResponseError.ResponseError{
			ServerError: motmedelErrors.NewWithTrace(nil_error.New("host to mux specification")),
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
			ServerError: motmedelErrors.NewWithTrace(nil_error.New("mux specification")),
		}
	}

	if redirectTo := muxSpecification.RedirectTo; redirectTo != "" {
		return &muxTypesResponse.Response{
			StatusCode: http.StatusMovedPermanently,
			Headers: []*muxTypesResponse.HeaderEntry{
				{Name: "Location", Value: motmedelStrings.HexEscapeNonASCII(redirectTo + request.RequestURI)},
			},
		}, nil
	} else if muxSpecificationMux := muxSpecification.Mux; muxSpecificationMux != nil {
		muxSpecificationMux.ServeHTTP(responseWriter, request)
		return nil, nil
	}

	return nil, &muxTypesResponseError.ResponseError{
		ServerError: motmedelErrors.NewWithTrace(
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
