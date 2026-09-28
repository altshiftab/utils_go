package token_header_extractor

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/header_extractor"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/header_extractor/header_extractor_config"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/token_header_extractor/token_header_extractor_config"
	muxResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/www_authenticate"
)

type Parser struct {
	Name   string
	config *token_header_extractor_config.Config
}

// Challenge answers for a token carried as "Authorization: Bearer", the one scheme whose challenge
// parameters are defined (RFC 6750). Any other header or prefix has no challenge to give.
func (p *Parser) Challenge(errorCode string, errorDescription string) string {
	if !strings.EqualFold(p.config.HeaderName, "Authorization") {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(p.config.HeaderValuePrefix), "Bearer") {
		return ""
	}
	return www_authenticate.Bearer(errorCode, errorDescription)
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
		p.addChallenge(request, responseError)
		return "", responseError
	}

	headerValue = strings.TrimPrefix(headerValue, p.config.HeaderValuePrefix)

	return headerValue, nil
}

// addChallenge puts a challenge on a refusal answered with 401, which HTTP requires to carry one: no
// header is a request with no token, and more than one is a malformed request.
func (p *Parser) addChallenge(request *http.Request, responseError *response_error.ResponseError) {
	problemDetail := responseError.ProblemDetail
	if problemDetail == nil || problemDetail.Status != http.StatusUnauthorized || request == nil {
		return
	}

	var challenge string
	if len(request.Header.Values(p.config.HeaderName)) > 1 {
		challenge = p.Challenge(www_authenticate.ErrorInvalidRequest, p.config.ProblemDetailMultipleText)
	} else {
		challenge = p.Challenge("", "")
	}
	if challenge == "" {
		return
	}

	responseError.Headers = append(
		responseError.Headers,
		&muxResponse.HeaderEntry{Name: www_authenticate.HeaderName, Value: challenge},
	)
}

func New(options ...token_header_extractor_config.Option) *Parser {
	return &Parser{config: token_header_extractor_config.New(options...)}
}
