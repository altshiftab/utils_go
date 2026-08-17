package body_parser

import (
	"net/http"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	processorPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/processor"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/utils"
)

type BodyParser[T any] interface {
	Parse(*http.Request, []byte) (T, *response_error.ResponseError)
}

type BodyParserFunction[T any] func(*http.Request, []byte) (T, *response_error.ResponseError)

func (bpf BodyParserFunction[T]) Parse(request *http.Request, body []byte) (T, *response_error.ResponseError) {
	return bpf(request, body)
}

func New[T any](f func(*http.Request, []byte) (T, *response_error.ResponseError)) BodyParser[T] {
	return BodyParserFunction[T](f)
}

type BodyParserWithProcessor[T any, U any] struct {
	BodyParser BodyParser[T]
	Processor  processorPkg.Processor[U, T]
}

func (p *BodyParserWithProcessor[T, U]) Parse(request *http.Request, body []byte) (U, *response_error.ResponseError) {
	var zero U

	bodyParser := p.BodyParser
	if utils.IsNil(bodyParser) {
		return zero, &response_error.ResponseError{ServerError: motmedelErrors.NewWithTrace(nil_error.New("body parser"))}
	}

	processor := p.Processor
	if utils.IsNil(processor) {
		return zero, &response_error.ResponseError{ServerError: motmedelErrors.NewWithTrace(nil_error.New("processor"))}
	}

	result, responseError := bodyParser.Parse(request, body)
	if responseError != nil {
		return zero, responseError
	}

	processedResult, responseError := processor.Process(request.Context(), result)
	if responseError != nil {
		return zero, responseError
	}

	return processedResult, nil
}

func NewWithProcessor[T any, U any](bodyParser BodyParser[T], processor processorPkg.Processor[U, T]) *BodyParserWithProcessor[T, U] {
	return &BodyParserWithProcessor[T, U]{
		BodyParser: bodyParser,
		Processor:  processor,
	}
}
