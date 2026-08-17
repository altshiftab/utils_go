package types

import (
	"errors"
	"fmt"
	"testing"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
)

var (
	errOther         = errors.New("x")
	errSchemaProblem = errors.New("schema problem")
)

func TestValidationErrorError(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name            string
		validationError *ValidationError
		want            string
	}{
		{
			name:            "no keyword location",
			validationError: &ValidationError{Message: "boom"},
			want:            "#: boom",
		},
		{
			name:            "with keyword location",
			validationError: &ValidationError{Message: "boom", KeywordLocation: "#/properties/a"},
			want:            "#/properties/a: boom",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := testCase.validationError.Error(); got != testCase.want {
				t.Errorf("Error() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestValidationErrorsError(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name             string
		validationErrors *ValidationErrors
		want             string
	}{
		{
			name: "single",
			validationErrors: &ValidationErrors{
				Errs: []*ValidationError{{Message: "one"}},
			},
			want: "#: one",
		},
		{
			name: "multiple joined",
			validationErrors: &ValidationErrors{
				Errs: []*ValidationError{{Message: "one"}, {Message: "two"}},
			},
			want: "#: one\n#: two",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := testCase.validationErrors.Error(); got != testCase.want {
				t.Errorf("Error() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestIsValidationError(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "validation error", err: &ValidationError{Message: "x"}, want: true},
		{name: "validation errors", err: &ValidationErrors{}, want: true},
		{name: "other error", err: errOther, want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := IsValidationError(testCase.err); got != testCase.want {
				t.Errorf("IsValidationError(%v) = %t, want %t", testCase.err, got, testCase.want)
			}
		})
	}
}

func TestAddError(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name                string
		initial             error
		add                 error
		loc                 string
		wantErrText         string
		wantKeywordLocation string
	}{
		{
			name:                "validation error into nil",
			add:                 &ValidationError{Message: "boom"},
			loc:                 "minimum",
			wantErrText:         "#/minimum: boom",
			wantKeywordLocation: "#/minimum",
		},
		{
			name:                "prefixes existing location",
			add:                 &ValidationError{Message: "boom", KeywordLocation: "#/type"},
			loc:                 "properties/a",
			wantErrText:         "#/properties/a/type: boom",
			wantKeywordLocation: "#/properties/a/type",
		},
		{
			name:                "empty location keeps existing",
			add:                 &ValidationError{Message: "boom", KeywordLocation: "#/type"},
			loc:                 "",
			wantErrText:         "#/type: boom",
			wantKeywordLocation: "#/type",
		},
		{
			name:        "collects into validation errors",
			initial:     &ValidationError{Message: "first"},
			add:         &ValidationError{Message: "second"},
			loc:         "",
			wantErrText: "#: first\n#: second",
		},
		{
			name:        "flattens validation errors",
			add:         &ValidationErrors{Errs: []*ValidationError{{Message: "a"}, {Message: "b"}}},
			loc:         "allOf/0",
			wantErrText: "#/allOf/0: a\n#/allOf/0: b",
		},
		{
			name:        "non-validation error replaces validation error",
			initial:     &ValidationError{Message: "validation"},
			add:         errSchemaProblem,
			loc:         "",
			wantErrText: "schema problem",
		},
		{
			name:        "nil add keeps initial",
			initial:     &ValidationError{Message: "kept"},
			add:         nil,
			loc:         "ignored",
			wantErrText: "#: kept",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := testCase.initial
			AddError(&err, testCase.add, testCase.loc)

			if err == nil {
				t.Fatal("resulting error is nil")
			}
			if got := err.Error(); got != testCase.wantErrText {
				t.Errorf("error text = %q, want %q", got, testCase.wantErrText)
			}

			if testCase.wantKeywordLocation != "" {
				validationError, ok := errors.AsType[*ValidationError](err)
				if !ok {
					t.Fatalf("error is %T, want *ValidationError", err)
				}
				if validationError.KeywordLocation != testCase.wantKeywordLocation {
					t.Errorf("KeywordLocation = %q, want %q", validationError.KeywordLocation, testCase.wantKeywordLocation)
				}
			}
		})
	}
}

func TestAddValidationErrorStruct(t *testing.T) {
	t.Parallel()
	t.Run("nil to single to multiple", func(t *testing.T) {
		t.Parallel()
		var err error
		AddValidationErrorStruct(&err, &ValidationError{Message: "one"})
		if _, ok := errors.AsType[*ValidationError](err); !ok {
			t.Fatalf("after first add, error is %T, want *ValidationError", err)
		}

		AddValidationErrorStruct(&err, &ValidationError{Message: "two"})
		validationErrors, ok := errors.AsType[*ValidationErrors](err)
		if !ok {
			t.Fatalf("after second add, error is %T, want *ValidationErrors", err)
		}
		if len(validationErrors.Errs) != 2 {
			t.Errorf("len(Errs) = %d, want 2", len(validationErrors.Errs))
		}
	})

	t.Run("non-validation error is not disturbed", func(t *testing.T) {
		t.Parallel()
		err := errSchemaProblem
		AddValidationErrorStruct(&err, &ValidationError{Message: "one"})
		if err.Error() != "schema problem" {
			t.Errorf("error = %q, want %q", err.Error(), "schema problem")
		}
	})
}

func TestValidateError(t *testing.T) {
	t.Parallel()
	inner := &ValidationError{Message: "boom"}
	err := NewValidateError(inner, []*ValidationError{inner})

	validateError, ok := errors.AsType[*ValidateError](err)
	if !ok {
		t.Fatalf("NewValidateError returned %T, want *ValidateError", err)
	}
	if len(validateError.Errors) != 1 {
		t.Errorf("len(Errors) = %d, want 1", len(validateError.Errors))
	}

	if !errors.Is(err, motmedelErrors.ErrValidationError) {
		t.Error("errors.Is(err, motmedelErrors.ErrValidationError) = false, want true")
	}
	if errors.Is(err, motmedelErrors.ErrParseError) {
		t.Error("errors.Is(err, motmedelErrors.ErrParseError) = true, want false")
	}

	if unwrapped := errors.Unwrap(err); !errors.Is(unwrapped, inner) {
		t.Errorf("errors.Unwrap = %v, want %v", unwrapped, inner)
	}

	wrapped := fmt.Errorf("outer: %w", err)
	if !errors.Is(wrapped, motmedelErrors.ErrValidationError) {
		t.Error("wrapped errors.Is(ErrValidationError) = false, want true")
	}
}
