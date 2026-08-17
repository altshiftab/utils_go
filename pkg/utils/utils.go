package utils

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"reflect"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	motmedelContext "github.com/altshiftab/utils_go/pkg/context"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
)

func Convert[T any](value any) (T, error) {
	convertedValue, ok := value.(T)
	if !ok {
		return convertedValue, motmedelErrors.NewWithTrace(
			fmt.Errorf("%w: %T", motmedelErrors.ErrConversionNotOk, value),
			value,
		)
	}

	return convertedValue, nil
}

func ConvertSlice[T any](value any) ([]T, error) {
	var zero []T

	if typedValue, ok := value.([]T); ok {
		return typedValue, nil
	}

	if anySlice, ok := value.([]any); ok {
		typedSlice := make([]T, len(anySlice))
		for i, sliceValue := range anySlice {
			typedSliceValue, ok := sliceValue.(T)
			if !ok {
				return nil, motmedelErrors.NewWithTrace(
					fmt.Errorf("%w: %T (slice value)", motmedelErrors.ErrConversionNotOk, value),
					value,
				)
			}
			typedSlice[i] = typedSliceValue
		}
		return typedSlice, nil
	}

	return zero, motmedelErrors.NewWithTrace(
		fmt.Errorf("%w: %T", motmedelErrors.ErrConversionNotOk, value),
		value,
	)
}

func ConvertToNonZero[T comparable](value any) (T, error) {
	var zero T

	convertedValue, err := Convert[T](value)
	if err != nil {
		return zero, fmt.Errorf("convert: %w", err)
	}

	if convertedValue == zero {
		return zero, motmedelErrors.NewWithTrace(motmedelErrors.ErrZeroValue)
	}

	return convertedValue, nil
}

func IsNil(value any) bool {
	return value == nil || (reflect.ValueOf(value).Kind() == reflect.Pointer && reflect.ValueOf(value).IsNil())
}

func Must(err error, label string) {
	if err != nil {
		slog.ErrorContext(
			motmedelContext.WithError(context.Background(), err),
			fmt.Sprintf("fatal: %s", label),
		)
		os.Exit(1)
	}
}

func GetContextValue[T any](ctx context.Context, key any) (T, error) {
	return Convert[T](ctx.Value(key))
}

func GetNonZeroContextValue[T comparable](ctx context.Context, key any) (T, error) {
	return ConvertToNonZero[T](ctx.Value(key))
}

func MapGet[T comparable, U any](m map[T]U, key T) (U, error) {
	var zero U

	if m == nil {
		return zero, motmedelErrors.NewWithTrace(nil_error.New("map"))
	}

	v, ok := m[key]
	if !ok {
		return zero, motmedelErrors.NewWithTrace(motmedelErrors.ErrNotInMap)
	}

	return v, nil
}

func MapGetNonZero[T comparable, U comparable](m map[T]U, key T) (U, error) {
	var zero U

	v, err := MapGet[T, U](m, key)
	if err != nil {
		return v, fmt.Errorf("map get: %w", err)
	}
	if v == zero || IsNil(v) {
		return zero, motmedelErrors.NewWithTrace(motmedelErrors.ErrMapZeroValue)
	}

	return v, nil
}

func MapGetConvert[U any, T comparable](m map[T]any, key T) (U, error) {
	var zero U

	v, err := MapGet[T, any](m, key)
	if err != nil {
		return zero, fmt.Errorf("map get: %w", err)
	}

	cv, err := Convert[U](v)
	if err != nil {
		return zero, motmedelErrors.New(fmt.Errorf("convert: %w", err), v)
	}

	return cv, nil
}

func MapGetConvertSlice[U any, T comparable](m map[T]any, key T) ([]U, error) {
	v, err := MapGet[T, any](m, key)
	if err != nil {
		return nil, fmt.Errorf("map get: %w", err)
	}

	cv, err := ConvertSlice[U](v)
	if err != nil {
		return nil, motmedelErrors.New(fmt.Errorf("convert slice: %w", err), v)
	}

	return cv, nil
}

func MapGetConvertNonZero[U comparable, T comparable](m map[T]any, key T) (U, error) {
	var zero U

	v, err := MapGetConvert[U, T](m, key)
	if err != nil {
		return zero, fmt.Errorf("map get convert: %w", err)
	}
	if v == zero || IsNil(v) {
		return zero, motmedelErrors.NewWithTrace(motmedelErrors.ErrMapZeroValue)
	}

	return v, nil
}

func AnyNonZero[T comparable](vals ...T) bool {
	var zero T
	for _, v := range vals {
		if v != zero {
			return true
		}
	}
	return false
}
