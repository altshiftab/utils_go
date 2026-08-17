package header_extractor

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	motmedelHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/header_extractor/header_extractor_config"
	muxResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail/problem_detail_config"
	motmedelHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

type Parser struct {
	Name   string
	config *header_extractor_config.Config
}

func (p *Parser) Parse(request *http.Request) (string, *muxResponseError.ResponseError) {
	if request == nil {
		return "", &muxResponseError.ResponseError{
			ServerError: motmedelErrors.NewWithTrace(nil_error.New("request")),
		}
	}

	requestHeader := request.Header
	if requestHeader == nil {
		return "", &muxResponseError.ResponseError{
			ServerError: motmedelErrors.NewWithTrace(nil_error.New("request header")),
		}
	}

	name := p.Name
	if name == "" {
		return "", &muxResponseError.ResponseError{ServerError: motmedelErrors.NewWithTrace(empty_error.New("name"))}
	}

	headerValue, err := motmedelHttpUtils.GetSingleHeader(name, requestHeader)
	if err != nil {
		wrappedErr := motmedelErrors.NewWithTrace(fmt.Errorf("get single header: %w", err), name)

		if errors.Is(err, motmedelHttpErrors.ErrMissingHeader) {
			return "", &muxResponseError.ResponseError{
				ClientError: wrappedErr,
				ProblemDetail: problem_detail.New(
					p.config.ProblemDetailStatusCode,
					problem_detail_config.WithDetail(p.config.ProblemDetailMissingText),
					problem_detail_config.WithExtension(map[string]any{"header": name}),
				),
			}
		} else if errors.Is(err, motmedelHttpErrors.ErrMultipleHeaderValues) {
			return "", &muxResponseError.ResponseError{
				ClientError: wrappedErr,
				ProblemDetail: problem_detail.New(
					p.config.ProblemDetailStatusCode,
					problem_detail_config.WithDetail(p.config.ProblemDetailMultipleText),
					problem_detail_config.WithExtension(map[string]any{"header": name}),
				),
			}
		}
		return "", &muxResponseError.ResponseError{ServerError: wrappedErr}
	}

	return headerValue, nil
}

func New(name string, options ...header_extractor_config.Option) (*Parser, error) {
	if name == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("name"))
	}

	return &Parser{Name: name, config: header_extractor_config.New(options...)}, nil
}
