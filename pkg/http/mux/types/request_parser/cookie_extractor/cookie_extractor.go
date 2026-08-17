package cookie_extractor

import (
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/cookie_extractor/cookie_extractor_config"
	muxResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail/problem_detail_config"
)

type Parser struct {
	Name   string
	config *cookie_extractor_config.Config
}

func (p *Parser) Parse(request *http.Request) (string, *muxResponseError.ResponseError) {
	if request == nil {
		return "", &muxResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("request")),
		}
	}

	name := p.Name
	if name == "" {
		return "", &muxResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(empty_error.New("name")),
		}
	}

	cookie, err := request.Cookie(name)
	if err != nil {
		wrappedErr := altshiftErrors.NewWithTrace(fmt.Errorf("request cookie: %w", err), name)
		if errors.Is(err, http.ErrNoCookie) {
			return "", &muxResponseError.ResponseError{
				ClientError: wrappedErr,
				ProblemDetail: problem_detail.New(
					p.config.ProblemDetailStatusCode,
					problem_detail_config.WithDetail(p.config.ProblemDetailText),
					problem_detail_config.WithExtension(map[string]any{"cookie": name}),
				),
			}
		}
		return "", &muxResponseError.ResponseError{ServerError: wrappedErr}
	}
	if cookie == nil {
		return "", &muxResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("cookie")),
		}
	}

	return cookie.Value, nil
}

func New(name string, options ...cookie_extractor_config.Option) (*Parser, error) {
	if name == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("name"))
	}

	return &Parser{Name: name, config: cookie_extractor_config.New(options...)}, nil
}

func NewTokenCookieExtractor(name string, options ...cookie_extractor_config.Option) (*Parser, error) {
	return New(
		name,
		slices.Concat(
			[]cookie_extractor_config.Option{
				cookie_extractor_config.WithProblemDetailStatusCode(http.StatusUnauthorized),
				cookie_extractor_config.WithProblemDetailText("Missing token cookie."),
			},
			options,
		)...,
	)
}
