package race_request_parser

import (
	"context"
	"net/http"
	"sync"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/race_request_parser/race_request_parser_config"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/utils"
)

type Parser[T any] struct {
	Parsers              []request_parser.RequestParser[T]
	responseErrorsParser func([]*response_error.ResponseError) *response_error.ResponseError
}

func (p *Parser[T]) Parse(request *http.Request) (T, *response_error.ResponseError) {
	var zero T

	if request == nil {
		return zero, &response_error.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("http request")),
		}
	}

	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()

	request = request.WithContext(ctx)

	var waitGroup sync.WaitGroup

	parsedResults := make([]T, len(p.Parsers))
	parserResponseErrors := make([]*response_error.ResponseError, len(p.Parsers))

parserLoop:
	for i, parser := range p.Parsers {
		if utils.IsNil(parser) {
			parsedResults[i] = zero
			parserResponseErrors[i] = nil
			continue
		}

		select {
		case <-ctx.Done():
			break parserLoop
		default:
			waitGroup.Go(
				func() {
					result, responseError := parser.Parse(request)
					parsedResults[i] = result
					parserResponseErrors[i] = responseError

					if !utils.IsNil(result) {
						cancel()
					}
				},
			)
		}
	}

	waitGroup.Wait()

	for i := range p.Parsers {
		if !utils.IsNil(parsedResults[i]) {
			return parsedResults[i], parserResponseErrors[i]
		}
	}

	return zero, p.responseErrorsParser(parserResponseErrors)
}

func New[T any](parsers []request_parser.RequestParser[T], options ...race_request_parser_config.Option) *Parser[T] {
	config := race_request_parser_config.New(options...)
	return &Parser[T]{
		Parsers:              parsers,
		responseErrorsParser: config.ResponseErrorParser,
	}
}
