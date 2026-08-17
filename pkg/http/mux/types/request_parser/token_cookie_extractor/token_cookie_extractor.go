package token_cookie_extractor

import (
	"fmt"
	"net/http"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/cookie_extractor"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/cookie_extractor/cookie_extractor_config"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/token_cookie_extractor/token_cookie_extractor_config"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
)

type Parser struct {
	*token_cookie_extractor_config.Config
}

func (p *Parser) Parse(request *http.Request) (string, *response_error.ResponseError) {
	cookieExtractor, err := cookie_extractor.New(
		p.Name,
		cookie_extractor_config.WithProblemDetailStatusCode(p.ProblemDetailStatusCode),
		cookie_extractor_config.WithProblemDetailText(p.ProblemDetailText),
	)
	if err != nil {
		return "", &response_error.ResponseError{ServerError: fmt.Errorf("header extractor new: %w", err)}
	}

	return cookieExtractor.Parse(request)
}

func New(options ...token_cookie_extractor_config.Option) *Parser {
	return &Parser{Config: token_cookie_extractor_config.New(options...)}
}
