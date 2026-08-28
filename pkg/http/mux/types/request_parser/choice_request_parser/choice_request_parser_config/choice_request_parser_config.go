// Package choice_request_parser_config holds the settings of an ordered-choice request parser.
package choice_request_parser_config

import (
	"net/http"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
)

func defaultResponseErrorsParser(responseErrors []*response_error.ResponseError) *response_error.ResponseError {
	for _, responseError := range responseErrors {
		if responseError != nil {
			return responseError
		}
	}

	return &response_error.ResponseError{ProblemDetail: problem_detail.New(http.StatusBadRequest)}
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
