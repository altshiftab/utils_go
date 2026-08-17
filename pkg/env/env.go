package env

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"

	context2 "github.com/altshiftab/utils_go/pkg/context"
	motmedelEnvErrors "github.com/altshiftab/utils_go/pkg/env/errors"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
)

func GetEnvWithDefault(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func ReadEnv(name string) (string, error) {
	value, found := os.LookupEnv(name)

	var err error
	if !found {
		err = motmedelErrors.NewWithTrace(fmt.Errorf("%w: %q", motmedelEnvErrors.ErrNotPresent, name), name)
	} else if value == "" {
		err = motmedelErrors.NewWithTrace(fmt.Errorf("%w: %q", empty_error.New("environment variable"), name), name)
	}

	if err != nil {
		return "", err
	}

	return value, nil
}

func ReadEnvFatalCtx(ctx context.Context, name string) string {
	value, err := ReadEnv(name)
	if err != nil {
		slog.ErrorContext(
			context2.WithError(ctx, err),
			"An environment variable could not be obtained.",
		)
		os.Exit(1)
	}

	return value
}

func ReadEnvFatal(name string) string {
	return ReadEnvFatalCtx(context.Background(), name)
}

func PopEnv(name string) (string, error) {
	value, err := ReadEnv(name)
	if err != nil {
		return "", err
	}

	if unsetErr := os.Unsetenv(name); unsetErr != nil {
		return "", motmedelErrors.NewWithTrace(fmt.Errorf("os unsetenv: %w", unsetErr))
	}

	return value, nil
}

func PopEnvFatalCtx(ctx context.Context, name string) string {
	value, err := PopEnv(name)
	if err != nil {
		slog.ErrorContext(
			context2.WithError(ctx, err),
			"An environment variable could not be popped.",
		)
		os.Exit(1)
	}

	return value
}

func PopEnvFatal(name string) string {
	return PopEnvFatalCtx(context.Background(), name)
}
