package redirector

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/redirector/redirector_config"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	motmedelStrings "github.com/altshiftab/utils_go/pkg/strings"
	"github.com/altshiftab/utils_go/pkg/utils"
)

var errMissingXForwardedProto = errors.New("missing X-Forwarded-Proto header")

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
			ServerError: motmedelErrors.NewWithTrace(nil_error.New("request parser")),
		}
	}

	if parser.RedirectUrl == nil {
		return zero, &response_error.ResponseError{
			ServerError: motmedelErrors.NewWithTrace(nil_error.New("url")),
		}
	}

	requestHeader := request.Header
	if requestHeader == nil {
		return zero, &response_error.ResponseError{
			ServerError: motmedelErrors.NewWithTrace(nil_error.New("request header")),
		}
	}

	host := request.Host
	if host == "" {
		return zero, &response_error.ResponseError{
			ServerError: motmedelErrors.NewWithTrace(empty_error.New("host")),
		}
	}

	requestUrl := request.URL
	if requestUrl == nil {
		return zero, &response_error.ResponseError{
			ServerError: motmedelErrors.NewWithTrace(nil_error.New("request url")),
		}
	}

	// TODO: Try `Forwarded` first, then `X-Forwarded-Proto`.
	scheme := requestHeader.Get("X-Forwarded-Proto")
	if scheme == "" {
		if parser.RequireProto {
			return zero, &response_error.ResponseError{
				ServerError: motmedelErrors.NewWithTrace(errMissingXForwardedProto),
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
			Value: motmedelStrings.HexEscapeNonASCII(redirectUrl.String()),
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
		return nil, motmedelErrors.NewWithTrace(nil_error.New("request parser"))
	}

	if redirectUrl == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("url"))
	}

	requestParserConfig := redirector_config.New(options...)

	return &Parser[T, S]{
		RequestParser:     requestParser,
		RedirectUrl:       redirectUrl,
		RedirectParameter: requestParserConfig.ParameterName,
		RequireProto:      requestParserConfig.RequireProto,
	}, nil
}
