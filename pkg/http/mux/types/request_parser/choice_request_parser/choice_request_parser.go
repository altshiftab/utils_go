// Package choice_request_parser admits a request that any one of several parsers admits.
//
// It is an ordered choice: the parsers are tried, and the first of them in declaration order that
// admitted the request is the one whose result is returned. They are tried concurrently, which is
// an optimisation rather than the contract -- an endpoint reachable by a session or by a signed
// token should not pay for both checks in series.
//
// The concurrency does cost something worth knowing. By default the first parser to admit the
// request cancels the others, so a slower parser declared earlier may be cut off before it
// finishes and its result lost: the declared precedence is honoured among the parsers that
// completed, rather than guaranteed. WithExclusive removes the cancellation along with the reason
// for it, and precedence is then exact.
package choice_request_parser

import (
	"context"
	"errors"
	"net/http"
	"sync"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/choice_request_parser/choice_request_parser_config"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail/problem_detail_config"
	"github.com/altshiftab/utils_go/pkg/utils"
)

// ErrAmbiguousCredentials is a request more than one parser admitted, where the caller asked for
// exactly one.
var ErrAmbiguousCredentials = errors.New("more than one parser admitted the request")

type Parser[T any] struct {
	Parsers              []request_parser.RequestParser[T]
	responseErrorsParser func([]*response_error.ResponseError) *response_error.ResponseError
	exclusive            bool
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

					// Where exactly one parser may admit the request, the others have to finish:
					// whether a second would have admitted it is the thing being asked.
					if !utils.IsNil(result) && !p.exclusive {
						cancel()
					}
				},
			)
		}
	}

	waitGroup.Wait()

	if p.exclusive {
		admitted := 0
		for i := range p.Parsers {
			if !utils.IsNil(parsedResults[i]) {
				admitted++
			}
		}

		if admitted > 1 {
			return zero, &response_error.ResponseError{
				ClientError: altshiftErrors.NewWithTrace(ErrAmbiguousCredentials, admitted),
				ProblemDetail: problem_detail.New(
					http.StatusBadRequest,
					problem_detail_config.WithDetail("The request carries more than one kind of credential."),
				),
			}
		}
	}

	for i := range p.Parsers {
		if !utils.IsNil(parsedResults[i]) {
			return parsedResults[i], parserResponseErrors[i]
		}
	}

	return zero, p.responseErrorsParser(parserResponseErrors)
}

func New[T any](parsers []request_parser.RequestParser[T], options ...choice_request_parser_config.Option) *Parser[T] {
	config := choice_request_parser_config.New(options...)
	return &Parser[T]{
		Parsers:              parsers,
		responseErrorsParser: config.ResponseErrorParser,
		exclusive:            config.Exclusive,
	}
}
