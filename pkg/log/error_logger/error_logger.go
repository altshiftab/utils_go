package error_logger

import (
	"fmt"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftLog "github.com/altshiftab/utils_go/pkg/log"
	altshiftContextLogger "github.com/altshiftab/utils_go/pkg/log/context_logger"
	altshiftLogError "github.com/altshiftab/utils_go/pkg/log/error"
	"log/slog"
)

type Logger struct {
	*slog.Logger
}

func (logger *Logger) Error(message string, err error, input ...any) {
	altshiftLogError.LogError(message, altshiftErrors.New(err, input...), logger.Logger)
}

func (logger *Logger) ErrorWithSkippingMessage(message string, err error, input ...any) {
	altshiftLogError.LogError(
		fmt.Sprintf("%s Skipping.", message),
		altshiftErrors.New(err, input...),
		logger.Logger,
	)
}

func (logger *Logger) Warning(message string, err error, input ...any) {
	altshiftLogError.LogWarning(message, altshiftErrors.New(err, input...), logger.Logger)
}

func (logger *Logger) Fatal(message string, err error, input ...any) {
	altshiftLogError.LogFatalWithExitCode(message, altshiftErrors.New(err, input...), logger.Logger, 1)
}

func (logger *Logger) FatalWithExitingMessage(message string, err error, input ...any) {
	altshiftLogError.LogFatal(
		fmt.Sprintf("%s Exiting.", message),
		altshiftErrors.New(err, input...),
		logger.Logger,
	)
}

func NewWithErrorContextExtractor(handler slog.Handler, extractor *altshiftLog.ErrorContextExtractor) *Logger {
	return &Logger{
		Logger: altshiftContextLogger.New(handler, extractor),
	}
}

func New(handler slog.Handler) *Logger {
	return NewWithErrorContextExtractor(handler, &altshiftLog.ErrorContextExtractor{})
}
