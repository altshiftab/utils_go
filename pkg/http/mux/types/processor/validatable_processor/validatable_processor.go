package validatable_processor

import (
	"context"
	errors2 "errors"
	"fmt"
	"net/http"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/processor"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail/problem_detail_config"
	"github.com/altshiftab/utils_go/pkg/interfaces/validatable"
	"github.com/altshiftab/utils_go/pkg/utils"
)

func New[T validatable.Validatable]() processor.Processor[T, T] {
	return processor.New(
		func(ctx context.Context, v T) (T, *response_error.ResponseError) {
			var zero T

			if err := ctx.Err(); err != nil {
				return zero, &response_error.ResponseError{ServerError: fmt.Errorf("context err: %w", err)}
			}

			if utils.IsNil(v) {
				return zero, nil
			}

			if err := v.Validate(); err != nil {
				wrappedErr := motmedelErrors.New(fmt.Errorf("validate: %w", err))
				if errors2.Is(wrappedErr, motmedelErrors.ErrValidationError) {
					return zero, &response_error.ResponseError{
						ClientError: err,
						ProblemDetail: problem_detail.New(
							http.StatusBadRequest,
							problem_detail_config.WithDetail("The body did not pass validation."),
						),
					}
				}
			}

			return v, nil
		},
	)
}
