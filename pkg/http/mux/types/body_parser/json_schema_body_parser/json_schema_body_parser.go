package json_schema_body_parser

import (
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"reflect"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/body_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/body_parser/json_body_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail/problem_detail_config"
	motmedelJsonSchema "github.com/altshiftab/utils_go/pkg/json/schema"
	"github.com/altshiftab/utils_go/pkg/utils"
)

var jsonMapBodyParser = json_body_parser.New[map[string]any]()
var jsonArrayBodyParser = json_body_parser.New[[]any]()

type Parser[T any] struct {
	schema     *motmedelJsonSchema.Schema
	bodyParser body_parser.BodyParser[T]
}

func (p *Parser[T]) Parse(request *http.Request, body []byte) (T, *response_error.ResponseError) {
	var zero T

	schema := p.schema
	if schema == nil {
		return zero, &response_error.ResponseError{ServerError: motmedelErrors.NewWithTrace(nil_error.New("schema"))}
	}

	bodyParser := p.bodyParser
	if utils.IsNil(bodyParser) {
		return zero, &response_error.ResponseError{ServerError: motmedelErrors.NewWithTrace(nil_error.New("body parser"))}
	}

	var data any
	var responseError *response_error.ResponseError

	if reflect.TypeFor[T]().Kind() == reflect.Slice {
		data, responseError = jsonArrayBodyParser.Parse(request, body)
	} else {
		data, responseError = jsonMapBodyParser.Parse(request, body)
	}
	if responseError != nil {
		return zero, responseError
	}

	if err := schema.Validate(data); err != nil {
		wrappedErr := motmedelErrors.New(fmt.Errorf("validate (input): %w", err), data, schema)

		if validateError, ok := errors.AsType[*motmedelJsonSchema.ValidateError](err); ok {
			return zero, &response_error.ResponseError{
				ProblemDetail: problem_detail.New(
					http.StatusUnprocessableEntity,
					problem_detail_config.WithDetail("Invalid body."),
					problem_detail_config.WithExtension(map[string]any{"errors": validateError.Errors}),
				),
				ClientError: wrappedErr,
			}
		}

		return zero, &response_error.ResponseError{ServerError: wrappedErr}
	}

	var result T
	result, parseResponseError := bodyParser.Parse(request, body)
	if parseResponseError != nil {
		return zero, parseResponseError
	}

	return result, nil
}

func NewWithSchema[T any](schema *motmedelJsonSchema.Schema) (*Parser[T], error) {
	if schema == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("schema"))
	}

	// The schema owns the unknown-member policy (additionalProperties), and validation runs before
	// unmarshaling; rejecting unknown members here would contradict schemas that permit them.
	return &Parser[T]{schema: schema, bodyParser: json_body_parser.New[T](jsonv2.RejectUnknownMembers(false))}, nil
}

func New[T any]() (*Parser[T], error) {
	schema, err := motmedelJsonSchema.NewFromType[T]()
	if err != nil {
		return nil, fmt.Errorf("schema new: %w", err)
	}

	return NewWithSchema[T](schema)
}
