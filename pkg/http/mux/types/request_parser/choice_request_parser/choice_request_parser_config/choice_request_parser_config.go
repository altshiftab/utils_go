// Package choice_request_parser_config holds the settings of an ordered-choice request parser.
package choice_request_parser_config

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	altshiftHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/http/types/www_authenticate"
)

// defaultResponseErrorsParser answers a request no parser admitted. A refusal of a credential that
// was there is chosen over one that found none: a client that sent a bearer token is told what was
// wrong with it, not that it sent no cookie. Where the refusal is a 401, it carries the challenges
// of the other 401s too, so that a client sending nothing learns every way it could have signed in.
func defaultResponseErrorsParser(responseErrors []*response_error.ResponseError) *response_error.ResponseError {
	var chosen *response_error.ResponseError
	for _, responseError := range responseErrors {
		if responseError != nil && !absentCredential(responseError) {
			chosen = responseError
			break
		}
	}
	if chosen == nil {
		for _, responseError := range responseErrors {
			if responseError != nil {
				chosen = responseError
				break
			}
		}
	}
	if chosen == nil {
		return &response_error.ResponseError{ProblemDetail: problem_detail.New(http.StatusBadRequest)}
	}

	return withChallenges(chosen, responseErrors)
}

// absentCredential reports whether a refusal is for a credential that was not sent at all, which
// says nothing about a credential that was.
func absentCredential(responseError *response_error.ResponseError) bool {
	clientError := responseError.ClientError
	return clientError != nil &&
		(errors.Is(clientError, http.ErrNoCookie) || errors.Is(clientError, altshiftHttpErrors.ErrMissingHeader))
}

func unauthorized(responseError *response_error.ResponseError) bool {
	return responseError.ProblemDetail != nil && responseError.ProblemDetail.Status == http.StatusUnauthorized
}

// withChallenges returns chosen with the WWW-Authenticate challenges of the other 401 refusals added,
// as a copy so that no parser's refusal is changed under it; chosen itself when there are none.
func withChallenges(chosen *response_error.ResponseError, responseErrors []*response_error.ResponseError) *response_error.ResponseError {
	if !unauthorized(chosen) {
		return chosen
	}

	present := map[string]struct{}{}
	for _, header := range chosen.Headers {
		if header != nil && strings.EqualFold(header.Name, www_authenticate.HeaderName) {
			present[header.Value] = struct{}{}
		}
	}

	var added []*response.HeaderEntry
	for _, responseError := range responseErrors {
		if responseError == nil || responseError == chosen || !unauthorized(responseError) {
			continue
		}
		for _, header := range responseError.Headers {
			if header == nil || !strings.EqualFold(header.Name, www_authenticate.HeaderName) {
				continue
			}
			if _, ok := present[header.Value]; ok {
				continue
			}
			present[header.Value] = struct{}{}
			added = append(added, header)
		}
	}
	if len(added) == 0 {
		return chosen
	}

	merged := *chosen
	merged.Headers = append(slices.Clone(chosen.Headers), added...)
	return &merged
}

type Config struct {
	ResponseErrorParser func([]*response_error.ResponseError) *response_error.ResponseError

	// Exclusive refuses a request that more than one parser admits.
	//
	// It is for authorization, where the question is not only whether a request may proceed but as
	// whom. A request carrying two kinds of credential has two answers to that, and picking one is
	// guessing at what the sender meant: an audit trail then records an identity nobody chose, and
	// a handler that grants more to one of them grants it on the strength of a declaration order.
	// OAuth 2.0 refuses the same thing for the same reason.
	//
	// It costs the early exit: every parser is run to completion, because whether a second would
	// have admitted the request is the thing being asked.
	Exclusive bool
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{
		ResponseErrorParser: defaultResponseErrorsParser,
	}
	for _, option := range options {
		option(config)
	}

	return config
}

// WithExclusive refuses a request that more than one parser admits.
func WithExclusive() Option {
	return func(config *Config) {
		config.Exclusive = true
	}
}

func WithResponseErrorParser(responseErrorParser func([]*response_error.ResponseError) *response_error.ResponseError) Option {
	return func(config *Config) {
		config.ResponseErrorParser = responseErrorParser
	}
}
