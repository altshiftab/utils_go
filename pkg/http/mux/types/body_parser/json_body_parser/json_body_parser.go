package json_body_parser

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"net/http"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail/problem_detail_config"
)

type Parser[T any] struct {
	options []jsonv2.Options
}

func (p *Parser[T]) Parse(_ *http.Request, body []byte) (T, *response_error.ResponseError) {
	var target T

	// Unknown members are rejected by default; caller-provided options come later and take precedence.
	options := jsonv2.JoinOptions(
		append([]jsonv2.Options{jsonv2.RejectUnknownMembers(true)}, p.options...)...,
	)

	if err := jsonv2.Unmarshal(body, &target, options); err != nil {
		wrappedErr := motmedelErrors.NewWithTrace(fmt.Errorf("json unmarshal: %w", err), body)

		if _, ok := errors.AsType[*jsontext.SyntacticError](err); ok {
			detail := "Invalid body. The body is not valid JSON."
			if errors.Is(err, jsontext.ErrDuplicateName) {
				detail = "Invalid body. Duplicate object member name."
			}

			return target, &response_error.ResponseError{
				ClientError: wrappedErr,
				ProblemDetail: problem_detail.New(
					http.StatusBadRequest,
					problem_detail_config.WithDetail(detail),
				),
			}
		}

		if semanticError, ok := errors.AsType[*jsonv2.SemanticError](err); ok {
			detail := "Invalid body. The value is not appropriate for the JSON type."
			if errors.Is(err, jsonv2.ErrUnknownName) {
				detail = fmt.Sprintf("Invalid body. Unknown member: %q.", string(semanticError.JSONPointer))
			}

			return target, &response_error.ResponseError{
				ClientError: wrappedErr,
				ProblemDetail: problem_detail.New(
					http.StatusUnprocessableEntity,
					problem_detail_config.WithDetail(detail),
				),
			}
		}

		return target, &response_error.ResponseError{ServerError: wrappedErr}
	}

	return target, nil
}

func New[T any](options ...jsonv2.Options) *Parser[T] {
	return &Parser[T]{options: options}
}
