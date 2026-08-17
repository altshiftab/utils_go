package error

import (
	"context"
	"fmt"

	"log/slog"
	"os"

	context2 "github.com/altshiftab/utils_go/pkg/context"
)

func LogError(message string, err error, logger *slog.Logger, args ...any) {
	if logger == nil {
		logger = slog.Default()
	}
	logger.ErrorContext(
		context2.WithError(context.Background(), err),
		message,
		args...,
	)
}

func LogWarning(message string, err error, logger *slog.Logger, args ...any) {
	if logger == nil {
		logger = slog.Default()
	}
	logger.WarnContext(
		context2.WithError(context.Background(), err),
		message,
		args...,
	)
}

func LogDebug(message string, err error, logger *slog.Logger, args ...any) {
	if logger == nil {
		logger = slog.Default()
	}
	logger.DebugContext(
		context2.WithError(context.Background(), err),
		message,
		args...,
	)
}

func LogFatalWithExitCode(message string, err error, logger *slog.Logger, exitCode int, args ...any) {
	if logger == nil {
		logger = slog.Default()
	}
	logger.ErrorContext(
		context2.WithError(context.Background(), err),
		message,
		args...,
	)
	os.Exit(exitCode)
}

func LogFatalWithExitingMessage(message string, err error, logger *slog.Logger, args ...any) {
	LogFatalWithExitCode(fmt.Sprintf("%s Exiting.", message), err, logger, 1, args...)
}

func LogFatal(message string, err error, logger *slog.Logger, args ...any) {
	LogFatalWithExitCode(message, err, logger, 1, args...)
}
