package jsonpointer_test

import (
	"encoding/json/v2"
	"testing"

	"github.com/altshiftab/utils_go/pkg/json/schema/draft202012"
	"github.com/altshiftab/utils_go/pkg/json/schema/jsonpointer"
	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

const rootSchemaJSON = `{
	"$schema": "https://json-schema.org/draft/2020-12/schema",
	"$defs": {
		"name": {"type": "string"},
		"a~b/c": {"type": "integer"},
		"thing": {
			"type": "object",
			"properties": {
				"inner": {"type": "boolean"}
			}
		}
	},
	"properties": {
		"foo": {"type": "number"}
	},
	"allOf": [
		{"type": "array"},
		{"type": "object"}
	],
	"not": {"type": "null"},
	"unknown": {"type": "string"}
}`

func TestDerefSchema(t *testing.T) {
	t.Parallel()

	var root schema.Schema
	if err := json.Unmarshal([]byte(rootSchemaJSON), &root); err != nil {
		t.Fatalf("failed to unmarshal root schema: %v", err)
	}

	testCases := []struct {
		name     string
		pointer  string
		wantRoot bool
		wantType string
		wantErr  bool
	}{
		{
			name:     "empty pointer returns root",
			pointer:  "",
			wantRoot: true,
		},
		{
			name:     "defs key",
			pointer:  "/$defs/name",
			wantType: "string",
		},
		{
			name:     "escaped defs key",
			pointer:  "/$defs/a~0b~1c",
			wantType: "integer",
		},
		{
			name:     "nested pointer through defs and properties",
			pointer:  "/$defs/thing/properties/inner",
			wantType: "boolean",
		},
		{
			name:     "properties key",
			pointer:  "/properties/foo",
			wantType: "number",
		},
		{
			name:     "allOf index 0",
			pointer:  "/allOf/0",
			wantType: "array",
		},
		{
			name:     "allOf index 1",
			pointer:  "/allOf/1",
			wantType: "object",
		},
		{
			name:     "not single schema",
			pointer:  "/not",
			wantType: "null",
		},
		{
			name:     "unknown keyword object value",
			pointer:  "/unknown",
			wantType: "string",
		},
		{
			name:    "nonexistent element",
			pointer: "/nosuch",
			wantErr: true,
		},
		{
			name:    "out of range array index",
			pointer: "/allOf/9",
			wantErr: true,
		},
		{
			name:    "non-integer array index",
			pointer: "/allOf/x",
			wantErr: true,
		},
		{
			name:    "missing properties key",
			pointer: "/properties/nosuch",
			wantErr: true,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := jsonpointer.DerefSchema(draft202012.SchemaID, &root, testCase.pointer)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("DerefSchema(%q) = %v, want error", testCase.pointer, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("DerefSchema(%q) returned unexpected error: %v", testCase.pointer, err)
			}
			if got == nil {
				t.Fatalf("DerefSchema(%q) returned nil schema", testCase.pointer)
			}

			if testCase.wantRoot {
				if got != &root {
					t.Errorf("DerefSchema(%q) = %v, want root schema", testCase.pointer, got)
				}
				return
			}

			value, ok := got.LookupKeyword("type")
			if !ok {
				t.Fatalf(`DerefSchema(%q) result has no "type" keyword: %v`, testCase.pointer, got)
			}
			stringOrStrings, ok := value.(schema.PartStringOrStrings)
			if !ok {
				t.Fatalf(`DerefSchema(%q) "type" value is %T, want schema.PartStringOrStrings`, testCase.pointer, value)
			}
			if stringOrStrings.String != testCase.wantType {
				t.Errorf(`DerefSchema(%q) "type" = %q, want %q`, testCase.pointer, stringOrStrings.String, testCase.wantType)
			}
		})
	}
}
