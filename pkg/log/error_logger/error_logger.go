package error_logger

import (
	"fmt"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	motmedelLog "github.com/altshiftab/utils_go/pkg/log"
	motmedelContextLogger "github.com/altshiftab/utils_go/pkg/log/context_logger"
	motmedelLogError "github.com/altshiftab/utils_go/pkg/log/error"
	"log/slog"
)

type Logger struct {
	*slog.Logger
}

func (logger *Logger) Error(message string, err error, input ...any) {
	motmedelLogError.LogError(message, motmedelErrors.New(err, input...), logger.Logger)
}

func (logger *Logger) ErrorWithSkippingMessage(message string, err error, input ...any) {
	motmedelLogError.LogError(
		fmt.Sprintf("%s Skipping.", message),
		motmedelErrors.New(err, input...),
		logger.Logger,
	)
}

func (logger *Logger) Warning(message string, err error, input ...any) {
	motmedelLogError.LogWarning(message, motmedelErrors.New(err, input...), logger.Logger)
}

func (logger *Logger) Fatal(message string, err error, input ...any) {
	motmedelLogError.LogFatalWithExitCode(message, motmedelErrors.New(err, input...), logger.Logger, 1)
}

func (logger *Logger) FatalWithExitingMessage(message string, err error, input ...any) {
	motmedelLogError.LogFatal(
		fmt.Sprintf("%s Exiting.", message),
		motmedelErrors.New(err, input...),
		logger.Logger,
	)
}

func NewWithErrorContextExtractor(handler slog.Handler, extractor *motmedelLog.ErrorContextExtractor) *Logger {
	return &Logger{
		Logger: motmedelContextLogger.New(handler, extractor),
	}
}

func New(handler slog.Handler) *Logger {
	return NewWithErrorContextExtractor(handler, &motmedelLog.ErrorContextExtractor{})
}
