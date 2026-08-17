// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

import (
	"errors"
	"fmt"
	"strings"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// ErrInvalidSchema indicates that a schema document itself is invalid:
// unparseable JSON, a keyword argument of the wrong type, or an
// unresolvable reference. It is not a validation failure of an
// instance, and deliberately does not match the Motmedel client-input
// error categories: an invalid schema is a programming error.
var ErrInvalidSchema = errors.New("invalid schema")

// ValidationError is returned by a validation function
// when an instance fails validation.
type ValidationError struct {
	// Basic output fields per JSON Schema output format (basic):
	// https://json-schema.org/draft/2020-12/json-schema-core.html#name-output-formats
	// These are the canonical fields consumers should use.
	Message          string `json:"error"`
	KeywordLocation  string `json:"keywordLocation"`
	InstanceLocation string `json:"instanceLocation"`
}

// Error returns the error message that a user should see.
// This implements the error interface.
func (ve *ValidationError) Error() string {
	kl := ve.KeywordLocation
	if kl == "" {
		kl = "#"
	}
	return fmt.Sprintf("%s: %s", kl, ve.Message)
}

// ValidationErrors is a collection of ValidationError values.
//
//nolint:errname // Established API name for an aggregation of validation errors.
type ValidationErrors struct {
	Errs []*ValidationError
}

// Error returns the error message that a user should see.
// This implements the error interface.
func (ves *ValidationErrors) Error() string {
	if len(ves.Errs) == 1 {
		return ves.Errs[0].Error()
	}
	errs := make([]error, len(ves.Errs))
	for i, ve := range ves.Errs {
		errs[i] = ve
	}
	return errors.Join(errs...).Error()
}

// ValidateError is the error returned by [Schema.Validate] when an
// instance fails validation. It wraps the underlying error and collects
// the individual validation errors.
type ValidateError struct {
	error
	Errors []*ValidationError
}

// Unwrap returns the wrapped error.
func (ve *ValidateError) Unwrap() error {
	return ve.error
}

// Is marks a ValidateError as a client-input validation failure,
// so that errors.Is(err, motmedelErrors.ErrValidationError) reports true.
func (ve *ValidateError) Is(target error) bool {
	return target == motmedelErrors.ErrValidationError
}

// NewValidateError returns a [*ValidateError] wrapping err and
// collecting errs.
func NewValidateError(err error, errs []*ValidationError) error {
	return &ValidateError{error: err, Errors: errs}
}

// IsValidationError reports whether err is a validation error.
// This deliberately matches only direct validation error values;
// validation errors are aggregated, not wrapped.
func IsValidationError(err error) bool {
	//nolint:errorlint // Validation errors are aggregated, not wrapped; direct type matching is deliberate.
	switch err.(type) {
	case *ValidationError, *ValidationErrors:
		return true
	}
	return false
}

// AddError adds an error, which may be a validation error,
// to another error.
func AddError(perr *error, err error, loc string) {
	if err == nil {
		return
	}

	//nolint:errorlint // Validation errors are aggregated, not wrapped; direct type matching is deliberate.
	if ve, ok := err.(*ValidationError); ok {
		// Build a combined keywordLocation by prefixing the provided loc
		// to any existing keywordLocation, using JSON Pointer rules.
		// Start from existing pointer tail (without leading '#').
		tail := ""
		if ve.KeywordLocation != "" {
			tl := ve.KeywordLocation
			if strings.HasPrefix(tl, "#/") {
				tail = tl[2:]
			} else if tl == "#" {
				tail = ""
			} else if strings.HasPrefix(tl, "#") {
				tail = tl[1:]
			} else {
				// Not a pointer, treat as raw tail
				tail = tl
			}
		}

		// Compose: loc (if any) + tail (if any)
		var composed string
		switch {
		case loc == "" && tail == "":
			composed = "#"
		case loc == "":
			composed = "#/" + tail
		case tail == "":
			composed = "#/" + loc
		default:
			composed = "#/" + loc + "/" + tail
		}

		nev := &ValidationError{
			Message:         ve.Message,
			KeywordLocation: composed,
			InstanceLocation: func() string {
				if ve.InstanceLocation == "" {
					return "#"
				}
				return ve.InstanceLocation
			}(),
		}
		AddValidationErrorStruct(perr, nev)
		return
	}
	//nolint:errorlint // Validation errors are aggregated, not wrapped; direct type matching is deliberate.
	if ves, ok := err.(*ValidationErrors); ok {
		for _, ve := range ves.Errs {
			// Handle each inner error through AddError logic recursively.
			AddError(perr, ve, loc)
		}
		return
	}

	// The new error is not a validation error.

	//nolint:errorlint // Validation errors are aggregated, not wrapped; direct type matching is deliberate.
	if _, ok := (*perr).(*ValidationError); ok {
		// Replace a validation error with a non-validation error.
		*perr = err
	} else if _, ok := (*perr).(*ValidationErrors); ok { //nolint:errorlint // Validation errors are aggregated, not wrapped; direct type matching is deliberate.
		*perr = err
	} else if unwrap, ok := (*perr).(interface{ Unwrap() []error }); ok && len(unwrap.Unwrap()) > 0 {
		*perr = errors.Join(append(unwrap.Unwrap(), err)...)
	} else {
		*perr = errors.Join(*perr, err)
	}
}

// AddValidationErrorStruct adds a [ValidationError] to an existing error.
// The provided ve should already have basic fields populated.
// An existing error that is not a validation error is not disturbed.
func AddValidationErrorStruct(perr *error, ve *ValidationError) {
	if *perr == nil {
		*perr = ve
	} else if one, ok := (*perr).(*ValidationError); ok { //nolint:errorlint // Validation errors are aggregated, not wrapped; direct type matching is deliberate.
		*perr = &ValidationErrors{
			Errs: []*ValidationError{
				one,
				ve,
			},
		}
	} else if ves, ok := (*perr).(*ValidationErrors); ok { //nolint:errorlint // Validation errors are aggregated, not wrapped; direct type matching is deliberate.
		ves.Errs = append(ves.Errs, ve)
	}
}
