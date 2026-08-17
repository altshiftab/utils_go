package cbor_schema_body_parser

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/altshiftab/utils_go/pkg/cbor"
	cborSchema "github.com/altshiftab/utils_go/pkg/cbor/schema"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail/problem_detail_config"
)

type Parser[T any] struct {
	schema *cborSchema.Schema
}

func (p *Parser[T]) Parse(_ *http.Request, body []byte) (T, *response_error.ResponseError) {
	var zero T

	schema := p.schema
	if schema == nil {
		return zero, &response_error.ResponseError{
			ServerError: motmedelErrors.NewWithTrace(cborSchema.ErrNilSchema),
		}
	}

	// The parsed result's byte strings alias body, which the mux keeps alive for the request
	// duration.
	value, err := cbor.DecodeNoCopy(body)
	if err != nil {
		return zero, &response_error.ResponseError{
			ClientError: motmedelErrors.NewWithTrace(fmt.Errorf("cbor decode: %w", err), body),
			ProblemDetail: problem_detail.New(
				http.StatusBadRequest,
				problem_detail_config.WithDetail("Malformed CBOR body."),
			),
		}
	}

	if err := schema.Validate(value); err != nil {
		wrappedErr := motmedelErrors.New(fmt.Errorf("validate (input): %w", err), value, schema)

		var validateError *cborSchema.ValidateError
		if errors.As(err, &validateError) {
			return zero, &response_error.ResponseError{
				ProblemDetail: problem_detail.New(
					http.StatusUnprocessableEntity,
					problem_detail_config.WithDetail("Invalid body."),
					problem_detail_config.WithExtension(map[string]any{"errors": validateError.Issues}),
				),
				ClientError: wrappedErr,
			}
		}

		return zero, &response_error.ResponseError{ServerError: wrappedErr}
	}

	var result T
	if err := cbor.UnmarshalValue(value, &result); err != nil {
		return zero, &response_error.ResponseError{
			ServerError: motmedelErrors.New(fmt.Errorf("cbor unmarshal value: %w", err), value),
		}
	}

	return result, nil
}

func NewWithSchema[T any](schema *cborSchema.Schema) (*Parser[T], error) {
	if schema == nil {
		return nil, motmedelErrors.NewWithTrace(cborSchema.ErrNilSchema)
	}

	return &Parser[T]{schema: schema}, nil
}

func New[T any]() (*Parser[T], error) {
	schema, err := cborSchema.NewFromType[T]()
	if err != nil {
		return nil, fmt.Errorf("schema new from type: %w", err)
	}

	return NewWithSchema[T](schema)
}
