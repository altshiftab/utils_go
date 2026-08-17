package token_header_extractor

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/header_extractor"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/header_extractor/header_extractor_config"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/token_header_extractor/token_header_extractor_config"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
)

type Parser struct {
	Name   string
	config *token_header_extractor_config.Config
}

func (p *Parser) Parse(request *http.Request) (string, *response_error.ResponseError) {
	headerExtractor, err := header_extractor.New(
		p.config.HeaderName,
		header_extractor_config.WithProblemDetailStatusCode(p.config.ProblemDetailStatusCode),
		header_extractor_config.WithProblemDetailMissingText(p.config.ProblemDetailMissingText),
		header_extractor_config.WithProblemDetailMultipleText(p.config.ProblemDetailMultipleText),
	)
	if err != nil {
		return "", &response_error.ResponseError{ServerError: fmt.Errorf("header extractor new: %w", err)}
	}

	headerValue, responseError := headerExtractor.Parse(request)
	if responseError != nil {
		return "", responseError
	}

	headerValue = strings.TrimPrefix(headerValue, p.config.HeaderValuePrefix)

	return headerValue, nil
}

func New(options ...token_header_extractor_config.Option) *Parser {
	return &Parser{config: token_header_extractor_config.New(options...)}
}
