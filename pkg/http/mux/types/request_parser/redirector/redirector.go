package redirector

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/forwarded_headers"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/redirector/redirector_config"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	altshiftStrings "github.com/altshiftab/utils_go/pkg/strings"
	"github.com/altshiftab/utils_go/pkg/utils"
)

var errMissingForwardedProto = errors.New("missing proto in the Forwarded and X-Forwarded-Proto headers")

type Parser[T request_parser.RequestParser[S], S any] struct {
	RequestParser     T
	RedirectUrl       *url.URL
	RedirectParameter string
	RequireProto      bool
}

func (parser *Parser[T, S]) Parse(request *http.Request) (S, *response_error.ResponseError) {
	var zero S

	requestParser := parser.RequestParser
	if utils.IsNil(requestParser) {
		return zero, &response_error.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("request parser")),
		}
	}

	if parser.RedirectUrl == nil {
		return zero, &response_error.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("url")),
		}
	}

	requestHeader := request.Header
	if requestHeader == nil {
		return zero, &response_error.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("request header")),
		}
	}

	// The vhost mux in front resolves which host the request is for and carries
	// it on the request; behind a proxy that rewrites Host, that is the only
	// place the name the client used survives. Without one -- a service serving
	// a single host, a development server -- there is nothing to read and the
	// request answers for itself.
	host, ok := forwarded_headers.AuthorityFromContext(request.Context())
	if !ok {
		host = request.Host
	}
	if host == "" {
		return zero, &response_error.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(empty_error.New("host")),
		}
	}

	requestUrl := request.URL
	if requestUrl == nil {
		return zero, &response_error.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("request url")),
		}
	}

	scheme := forwarded_headers.Scheme(requestHeader)
	if scheme == "" {
		if parser.RequireProto {
			return zero, &response_error.ResponseError{
				ServerError: altshiftErrors.NewWithTrace(errMissingForwardedProto),
			}
		}

		if request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}

	out, responseError := requestParser.Parse(request)
	if responseError == nil {
		return out, nil
	}

	if requestHeader.Get("Sec-Fetch-Mode") != "navigate" {
		return zero, responseError
	}

	problemDetail := responseError.ProblemDetail
	if problemDetail == nil {
		return zero, responseError
	}

	if problemDetail.Status != http.StatusUnauthorized {
		return zero, responseError
	}

	redirectParameter := parser.RedirectParameter
	if redirectParameter == "" {
		redirectParameter = redirector_config.DefaultParameterName
	}

	currentUrl := *request.URL
	currentUrl.Host = host
	currentUrl.Scheme = scheme

	redirectUrl := *parser.RedirectUrl
	query := redirectUrl.Query()
	query.Set(redirectParameter, currentUrl.String())
	redirectUrl.RawQuery = query.Encode()

	responseError.Headers = append(
		responseError.Headers,
		&response.HeaderEntry{
			Name:  "Location",
			Value: altshiftStrings.HexEscapeNonASCII(redirectUrl.String()),
		},
	)

	return zero, responseError
}

func New[T request_parser.RequestParser[S], S any](
	requestParser T,
	redirectUrl *url.URL,
	options ...redirector_config.Option,
) (*Parser[T, S], error) {
	if utils.IsNil(requestParser) {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("request parser"))
	}

	if redirectUrl == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("url"))
	}

	requestParserConfig := redirector_config.New(options...)

	return &Parser[T, S]{
		RequestParser:     requestParser,
		RedirectUrl:       redirectUrl,
		RedirectParameter: requestParserConfig.ParameterName,
		RequireProto:      requestParserConfig.RequireProto,
	}, nil
}
