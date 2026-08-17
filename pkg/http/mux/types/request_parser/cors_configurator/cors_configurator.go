package cors_configurator

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail/problem_detail_config"
	"github.com/altshiftab/utils_go/pkg/interfaces/comparer"
	"github.com/altshiftab/utils_go/pkg/net/types/domain_parts"
	"github.com/altshiftab/utils_go/pkg/utils"
)

type Configurator struct {
	AllowedOrigins         []string
	AllowedOriginsComparer comparer.Comparer[string]
	RegisteredDomain       string

	Headers       []string
	Credentials   bool
	MaxAge        int
	ExposeHeaders []string
}

func (c *Configurator) Parse(request *http.Request) (*altshiftHttpTypes.CorsConfiguration, *response_error.ResponseError) {
	if request == nil {
		return nil, &response_error.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("request")),
		}
	}

	requestHeader := request.Header
	if requestHeader == nil {
		return nil, &response_error.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("request header")),
		}
	}

	origin := requestHeader.Get("Origin")
	if origin == "" {
		return nil, nil
	}

	var matchedAllowedOrigin string
	for _, allowedOrigin := range c.AllowedOrigins {
		if strings.EqualFold(origin, allowedOrigin) {
			matchedAllowedOrigin = allowedOrigin
			break
		}
	}

	if matchedAllowedOrigin == "" && !utils.IsNil(c.AllowedOriginsComparer) {
		if ok, _ := c.AllowedOriginsComparer.Compare(origin); ok {
			matchedAllowedOrigin = origin
		}
	}

	registeredDomain := c.RegisteredDomain
	if matchedAllowedOrigin == "" && registeredDomain != "" {
		parsedOrigin, err := url.Parse(origin)
		if err != nil {
			return nil, &response_error.ResponseError{
				ClientError: altshiftErrors.NewWithTrace(fmt.Errorf("url parse (origin): %w", err), origin),
				ProblemDetail: problem_detail.New(
					http.StatusBadRequest,
					problem_detail_config.WithDetail("Invalid Origin header."),
				),
			}
		}

		originHostname := parsedOrigin.Hostname()
		originDomainParts := domain_parts.New(originHostname)
		if originDomainParts == nil {
			return nil, &response_error.ResponseError{
				ProblemDetail: problem_detail.New(
					http.StatusBadRequest,
					problem_detail_config.WithDetail("Invalid Origin header hostname."),
				),
			}
		}

		if strings.EqualFold(originDomainParts.RegisteredDomain, registeredDomain) {
			matchedAllowedOrigin = origin
		}
	}

	if matchedAllowedOrigin == "" {
		return nil, nil
	}

	return &altshiftHttpTypes.CorsConfiguration{
		Origin:        matchedAllowedOrigin,
		Headers:       c.Headers,
		Credentials:   c.Credentials,
		MaxAge:        c.MaxAge,
		ExposeHeaders: c.ExposeHeaders,
	}, nil
}
