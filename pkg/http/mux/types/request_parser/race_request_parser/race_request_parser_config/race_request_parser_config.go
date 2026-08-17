package race_request_parser_config

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

func WithResponseErrorParser(responseErrorParser func([]*response_error.ResponseError) *response_error.ResponseError) Option {
	return func(config *Config) {
		config.ResponseErrorParser = responseErrorParser
	}
}
