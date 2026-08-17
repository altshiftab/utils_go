package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	motmedelContext "github.com/altshiftab/utils_go/pkg/context"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	motmedelHttpContext "github.com/altshiftab/utils_go/pkg/http/context"
	muxContext "github.com/altshiftab/utils_go/pkg/http/mux/context"
	muxErrors "github.com/altshiftab/utils_go/pkg/http/mux/errors"
	muxTypesResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	muxTypesResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response_writer"
	motmedelHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

// The messages the mux logs an error response under. What is worth saying about one is in the HTTP
// context rather than here -- what was requested, and what came back -- so a log handler that reads
// that context may replace them with what it says. They are re-exported by the mux package, for a
// handler to match against by name rather than by a string written out a second time.
const (
	ClientErrorMessage    = "A client error occurred."
	ServerErrorMessage    = "A server error occurred."
	ResponseServedMessage = "An HTTP response was served."
)

// DefaultDoneCallback logs the response a mux has finished serving. It is logged at debug level,
// there being nothing wrong with a response that was served: what it is worth reading is the record
// of what was asked for and what came back, which a log handler reading the HTTP context adds.
//
// Whether anything comes of it is left to the level the logger is set to, rather than to a check of
// its own: a service whose logger is set to debug says so by being set to debug.
//
// The level is asked before the record is built rather than left to the logging call, which would
// build the attributes it is given and only then drop them. A service not logging at debug pays a
// level check per response and nothing else.
func DefaultDoneCallback(ctx context.Context) {
	if !slog.Default().Enabled(ctx, slog.LevelDebug) {
		return
	}

	slog.DebugContext(
		ctx,
		ResponseServedMessage,
		slog.Group(
			"event",
			slog.String("action", "http_response_served"),
			slog.String("reason", ResponseServedMessage),
		),
	)
}

func DefaultResponseErrorHandler(
	ctx context.Context,
	responseError *muxTypesResponseError.ResponseError,
	responseWriter *muxTypesResponse.ResponseWriter,
) {
	if responseError == nil {
		return
	}

	if responseWriter == nil {
		slog.ErrorContext(
			motmedelContext.WithError(
				ctx,
				motmedelErrors.NewWithTrace(nil_error.New("response writer")),
			),
			"The response writer is nil.",
		)
		return
	}

	var errorId string

	switch responseErrorType := responseError.Type(); responseErrorType {
	case muxTypesResponseError.ResponseErrorType_ClientError:
		defer func() {
			clientError := motmedelErrors.New(responseError.ClientError)
			clientError.Id = errorId
			slog.WarnContext(
				motmedelContext.WithError(ctx, clientError),
				ClientErrorMessage,
				slog.Group(
					"event",
					slog.String("reason", ClientErrorMessage),
					slog.String("action", "log_http_client_error"),
				),
			)
		}()
	case muxTypesResponseError.ResponseErrorType_ServerError:
		defer func() {
			serverError := motmedelErrors.New(responseError.ServerError)
			serverError.Id = errorId
			slog.ErrorContext(
				motmedelContext.WithError(ctx, serverError),
				ServerErrorMessage,
				slog.Group(
					"event",
					slog.String("reason", ServerErrorMessage),
					slog.String("action", "log_http_server_error"),
				),
			)
		}()
	case muxTypesResponseError.ResponseErrorType_Invalid:
		slog.ErrorContext(
			motmedelContext.WithError(
				ctx,
				motmedelErrors.NewWithTrace(muxErrors.ErrUnusableResponseError, responseError),
			),
			"An invalid response error type was encountered.",
		)
		return
	default:
		slog.ErrorContext(
			motmedelContext.WithError(
				ctx,
				motmedelErrors.NewWithTrace(
					fmt.Errorf("%w: %v", muxErrors.ErrUnexpectedResponseErrorType, responseErrorType),
				),
			),
			"An unexpected response error type was encountered.",
		)
		return
	}

	if responseWriter.WriteHeaderCalled {
		return
	}

	problemDetail, err := responseError.GetEffectiveProblemDetail()
	if err != nil {
		slog.ErrorContext(
			motmedelContext.WithError(
				ctx,
				motmedelErrors.NewWithTrace(
					fmt.Errorf("response error get effective problem detail: %w", err),
					responseError,
				),
			),
			"An error occurred when obtaining the effective response error problem detail.",
		)
		return
	}
	responseError.ProblemDetail = problemDetail
	errorId = problemDetail.Instance

	contentNegotiation, _ := ctx.Value(muxContext.ContentNegotiationContextKey).(*motmedelHttpTypes.ContentNegotiation)
	response, err := responseError.MakeResponse(contentNegotiation)
	if err != nil {
		slog.ErrorContext(
			motmedelContext.WithError(
				ctx,
				motmedelErrors.New(fmt.Errorf("make response error response: %w", err), responseError),
			),
			"An error occurred when making a response from a response error.",
		)
		return
	}

	var acceptEncoding *motmedelHttpTypes.AcceptEncoding
	if contentNegotiation != nil {
		acceptEncoding = contentNegotiation.AcceptEncoding
	}

	if err := responseWriter.WriteResponse(ctx, response, acceptEncoding); err != nil {
		slog.ErrorContext(
			motmedelContext.WithError(
				ctx,
				motmedelErrors.New(fmt.Errorf("write response: %w", err), responseError),
			),
			"An error occurred when writing an error response.",
		)
		return
	}

	if httpContext, ok := ctx.Value(motmedelHttpContext.HttpContextContextKey).(*motmedelHttpTypes.HttpContext); ok {
		httpContext.Response = &http.Response{
			StatusCode: responseWriter.WrittenStatusCode,
			Header:     responseWriter.Header(),
		}
		httpContext.ResponseBody = responseWriter.WrittenBody
	}
}
