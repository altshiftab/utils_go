package url_allower

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/mismatch_error"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/url_allower/url_allower_config"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail/problem_detail_config"
	"github.com/altshiftab/utils_go/pkg/interfaces/urler"
	motmedelNet "github.com/altshiftab/utils_go/pkg/net"
	"github.com/altshiftab/utils_go/pkg/net/types/domain_parts"
	"github.com/altshiftab/utils_go/pkg/utils"
)

type Parser[T urler.StringURLer] struct {
	request_parser.RequestParser[T]
	Config *url_allower_config.Config
}

func (p *Parser[T]) Parse(request *http.Request) (*url.URL, *response_error.ResponseError) {
	requestParser := p.RequestParser
	if utils.IsNil(requestParser) {
		return nil, &response_error.ResponseError{ServerError: motmedelErrors.NewWithTrace(nil_error.New("request parser"))}
	}

	result, responseError := requestParser.Parse(request)
	if responseError != nil {
		return nil, responseError
	}
	if utils.IsNil(result) {
		return nil, &response_error.ResponseError{ServerError: motmedelErrors.NewWithTrace(nil_error.New("string urler"))}
	}

	urlString := result.URL()
	if urlString == "" {
		return nil, &response_error.ResponseError{
			ProblemDetail: problem_detail.New(http.StatusBadRequest, problem_detail_config.WithDetail("Empty url.")),
		}
	}

	parsedUrl, err := url.Parse(urlString)
	if err != nil {
		return nil, &response_error.ResponseError{
			ProblemDetail: problem_detail.New(http.StatusBadRequest, problem_detail_config.WithDetail("Malformed url.")),
			ClientError:   motmedelErrors.NewWithTrace(fmt.Errorf("url parse: %w", err), urlString),
		}
	}

	config := p.Config
	if config == nil {
		return nil, &response_error.ResponseError{
			ServerError: motmedelErrors.NewWithTrace(nil_error.New("url processor config")),
		}
	}

	// Hostnames are case-insensitive; normalize before any comparison.
	parsedUrlHostname := strings.ToLower(parsedUrl.Hostname())

	// localhost and any *.localhost subdomain (RFC 6761) resolve to loopback; when
	// configured to allow localhost, let them through without domain validation.
	isAllowedLocalhost := config.AllowLocalhost && motmedelNet.IsLocalhost(parsedUrlHostname)

	if !isAllowedLocalhost {
		domainParts := domain_parts.New(parsedUrlHostname)
		if domainParts == nil {
			return nil, &response_error.ResponseError{
				ProblemDetail: problem_detail.New(
					http.StatusBadRequest,
					problem_detail_config.WithDetail("Malformed url hostname; not a domain."),
				),
				ClientError: motmedelErrors.NewWithTrace(nil_error.New("domain breakdown")),
			}
		}

		if len(config.AllowedDomains) > 0 || len(config.AllowedRegisteredDomains) > 0 {
			var allowed bool

			registeredDomain := domainParts.RegisteredDomain
			for _, domain := range config.AllowedRegisteredDomains {
				if domain == registeredDomain {
					allowed = true
					break
				}
			}

			if !allowed {
				for _, domain := range config.AllowedDomains {
					if domain == parsedUrlHostname {
						allowed = true
						break
					}
				}
			}

			if !allowed {
				return nil, &response_error.ResponseError{
					ClientError: mismatch_error.New(
						"redirect",
						config.AllowedRegisteredDomains,
						registeredDomain,
						config.AllowedDomains,
						parsedUrlHostname,
					),
					ProblemDetail: problem_detail.New(
						http.StatusBadRequest,
						problem_detail_config.WithDetail("The url hostname does not match any allowed domain."),
					),
				}
			}
		}
	}

	return parsedUrl, nil
}

func New[T urler.StringURLer](requestParser request_parser.RequestParser[T], options ...url_allower_config.Option) *Parser[T] {
	return &Parser[T]{
		RequestParser: requestParser,
		Config:        url_allower_config.New(options...),
	}
}
