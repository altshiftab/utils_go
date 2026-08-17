// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package argtype defines a few helpers for schema.ArgType.
// This is only used by the generator commands, not by user code.
package argtype

import (
	"fmt"

	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

// nameToString maps [types.ArgType] to a name used in generated
// function and type names.
var nameToString = map[schema.ArgType]string{
	schema.ArgTypeBool:             "Bool",
	schema.ArgTypeString:           "String",
	schema.ArgTypeStrings:          "Strings",
	schema.ArgTypeStringOrStrings:  "StringOrStrings",
	schema.ArgTypeInt:              "Int",
	schema.ArgTypeFloat:            "Float",
	schema.ArgTypeSchema:           "Schema",
	schema.ArgTypeSchemas:          "Schemas",
	schema.ArgTypeMapSchema:        "MapSchema",
	schema.ArgTypeSchemaOrSchemas:  "SchemaOrSchemas",
	schema.ArgTypeMapArrayOrSchema: "MapArrayOrSchema",
	schema.ArgTypeAny:              "Any",
}

// Name returns a name to use for a [types.ArgType] in
// generated function and type names.
func Name(sat schema.ArgType) string {
	if n, ok := nameToString[sat]; ok {
		return n
	}
	panic(fmt.Sprintf("unexpected ArgType value %d", sat))
}

// nameToGoType maps [types.ArgType] to the underlying Go type.
var nameToGoType = map[schema.ArgType]string{
	schema.ArgTypeBool:             "bool",
	schema.ArgTypeString:           "string",
	schema.ArgTypeStrings:          "[]string",
	schema.ArgTypeStringOrStrings:  "types.PartStringOrStrings",
	schema.ArgTypeInt:              "int64",
	schema.ArgTypeFloat:            "float64",
	schema.ArgTypeSchema:           "*types.Schema",
	schema.ArgTypeSchemas:          "[]*types.Schema",
	schema.ArgTypeMapSchema:        "map[string]*types.Schema",
	schema.ArgTypeSchemaOrSchemas:  "types.PartSchemaOrSchemas",
	schema.ArgTypeMapArrayOrSchema: "map[string]types.ArrayOrSchema",
	schema.ArgTypeAny:              "any",
}

// GoType returns the Go type of a [types.ArgType], as a string.
func GoType(sat schema.ArgType) string {
	if t, ok := nameToGoType[sat]; ok {
		return t
	}
	panic(fmt.Sprintf("unexpected Argtype %d", sat))
}
