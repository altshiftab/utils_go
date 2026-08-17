// Package schema validates JSON documents against JSON schemas,
// supporting draft 2020-12.
//
// This package is the entry point for common use: parsing a schema
// with [New] or deriving one from a Go type with [NewFromType], and
// validating instances with [Schema.Validate]. The subpackages hold
// the core representation (types), the draft 2020-12 vocabulary
// (draft202012), format validators (format), and schema construction
// (builder).
//
// The implementation derives from the Go team's jsonschema prototype;
// see the LICENSE file in this directory.
package schema

import (
	jsonv2 "encoding/json/v2"
	"fmt"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	_ "github.com/altshiftab/utils_go/pkg/json/schema/draft202012"
	_ "github.com/altshiftab/utils_go/pkg/json/schema/format"
	"github.com/altshiftab/utils_go/pkg/json/schema/types"
	altshiftReflect "github.com/altshiftab/utils_go/pkg/reflect"
	typeExportJsonschema "github.com/altshiftab/utils_go/pkg/type_export/jsonschema"
)

// Schema is a JSON schema.
type Schema = types.Schema

// ValidateError is the error returned by [Schema.Validate] when an
// instance fails validation. It matches
// [altshiftErrors.ErrValidationError] with errors.Is.
type ValidateError = types.ValidateError

// ValidationError is a single validation failure within a [ValidateError].
type ValidationError = types.ValidationError

// ErrInvalidSchema indicates that a schema document itself is invalid.
var ErrInvalidSchema = types.ErrInvalidSchema

// New parses a JSON schema.
func New(data []byte) (*Schema, error) {
	var s Schema
	if err := jsonv2.Unmarshal(data, &s); err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("json unmarshal: %w", err))
	}

	return &s, nil
}

// NewFromType derives a JSON schema from a Go type.
func NewFromType[T any]() (*Schema, error) {
	schemaData, err := typeExportJsonschema.Convert(altshiftReflect.TypeOf[T]())
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("type export jsonschema convert: %w", err))
	}

	parsedSchema, err := New([]byte(schemaData))
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("new: %w", err))
	}

	return parsedSchema, nil
}
