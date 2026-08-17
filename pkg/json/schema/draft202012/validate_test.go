package draft202012_test

import (
	"encoding/json/v2"
	"errors"
	"strings"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	_ "github.com/altshiftab/utils_go/pkg/json/schema/draft202012"
	_ "github.com/altshiftab/utils_go/pkg/json/schema/format"
	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

// mustUnmarshalSchema parses schemaJSON into a resolved schema.
func mustUnmarshalSchema(t *testing.T, schemaJSON string) *schema.Schema {
	t.Helper()
	var s schema.Schema
	if err := json.Unmarshal([]byte(schemaJSON), &s); err != nil {
		t.Fatalf("unmarshal schema %s: %v", schemaJSON, err)
	}
	return &s
}

// mustUnmarshalInstance parses instanceJSON the way an instance would
// normally be decoded.
func mustUnmarshalInstance(t *testing.T, instanceJSON string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(instanceJSON), &v); err != nil {
		t.Fatalf("unmarshal instance %s: %v", instanceJSON, err)
	}
	return v
}

// validationCase is one table entry for runValidationCases.
type validationCase struct {
	name string
	// The schema, as JSON. A "$schema" keyword is not required.
	schemaJSON string
	// The instance, as JSON.
	instanceJSON string
	// Whether validation is expected to fail.
	wantErr bool
	// If non-empty, the error text must contain this substring.
	wantErrContains string
	// If non-empty, some collected validation error must have this
	// instance location.
	wantInstanceLocation string
}

// runValidationCases runs a table of validation cases.
func runValidationCases(t *testing.T, testCases []validationCase) {
	t.Helper()
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s := mustUnmarshalSchema(t, testCase.schemaJSON)
			instance := mustUnmarshalInstance(t, testCase.instanceJSON)

			err := s.Validate(instance)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("Validate(%s) against %s: error %v, wantErr %t", testCase.instanceJSON, testCase.schemaJSON, err, testCase.wantErr)
			}
			if err == nil {
				return
			}

			if testCase.wantErrContains != "" && !strings.Contains(err.Error(), testCase.wantErrContains) {
				t.Errorf("Validate error %q does not contain %q", err.Error(), testCase.wantErrContains)
			}

			if testCase.wantInstanceLocation != "" {
				validateError, ok := errors.AsType[*schema.ValidateError](err)
				if !ok {
					t.Fatalf("Validate error is %T, want *schema.ValidateError", err)
				}
				found := false
				var locations []string
				for _, ve := range validateError.Errors {
					locations = append(locations, ve.InstanceLocation)
					if ve.InstanceLocation == testCase.wantInstanceLocation {
						found = true
					}
				}
				if !found {
					t.Errorf("no validation error with instance location %q, got %v", testCase.wantInstanceLocation, locations)
				}
			}
		})
	}
}

func TestValidateType(t *testing.T) {
	t.Parallel()
	runValidationCases(t, []validationCase{
		{name: "string ok", schemaJSON: `{"type": "string"}`, instanceJSON: `"x"`},
		{name: "string fail", schemaJSON: `{"type": "string"}`, instanceJSON: `5`, wantErr: true, wantErrContains: `instance has type "integer", want "string"`},
		{name: "integer ok", schemaJSON: `{"type": "integer"}`, instanceJSON: `5`},
		{name: "integer from float ok", schemaJSON: `{"type": "integer"}`, instanceJSON: `5.0`},
		{name: "integer fail", schemaJSON: `{"type": "integer"}`, instanceJSON: `5.5`, wantErr: true},
		{name: "number ok", schemaJSON: `{"type": "number"}`, instanceJSON: `5.5`},
		{name: "number fail", schemaJSON: `{"type": "number"}`, instanceJSON: `"5.5"`, wantErr: true},
		{name: "boolean ok", schemaJSON: `{"type": "boolean"}`, instanceJSON: `true`},
		{name: "boolean fail", schemaJSON: `{"type": "boolean"}`, instanceJSON: `0`, wantErr: true},
		{name: "null ok", schemaJSON: `{"type": "null"}`, instanceJSON: `null`},
		{name: "null fail", schemaJSON: `{"type": "null"}`, instanceJSON: `0`, wantErr: true},
		{name: "object ok", schemaJSON: `{"type": "object"}`, instanceJSON: `{}`},
		{name: "object fail", schemaJSON: `{"type": "object"}`, instanceJSON: `[]`, wantErr: true},
		{name: "array ok", schemaJSON: `{"type": "array"}`, instanceJSON: `[]`},
		{name: "array fail", schemaJSON: `{"type": "array"}`, instanceJSON: `{}`, wantErr: true},
		{name: "multiple types ok", schemaJSON: `{"type": ["string", "null"]}`, instanceJSON: `null`},
		{name: "multiple types fail", schemaJSON: `{"type": ["string", "null"]}`, instanceJSON: `5`, wantErr: true},
	})
}

func TestValidateTypeGoValues(t *testing.T) {
	t.Parallel()
	type role string

	testCases := []struct {
		name       string
		schemaJSON string
		instance   any
		wantErr    bool
	}{
		{name: "defined string type", schemaJSON: `{"type": "string"}`, instance: role("admin")},
		{name: "defined string type minLength", schemaJSON: `{"type": "string", "minLength": 10}`, instance: role("admin"), wantErr: true},
		{name: "slice of defined string type", schemaJSON: `{"type": "array", "items": {"type": "string"}}`, instance: []role{"a", "b"}},
		{name: "defined bool type", schemaJSON: `{"type": "boolean"}`, instance: true},
		{name: "int value", schemaJSON: `{"type": "integer"}`, instance: 5},
		{name: "uint value", schemaJSON: `{"type": "integer"}`, instance: uint8(5)},
		{name: "float value as number", schemaJSON: `{"type": "number"}`, instance: 5.5},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s := mustUnmarshalSchema(t, testCase.schemaJSON)
			err := s.Validate(testCase.instance)
			if (err != nil) != testCase.wantErr {
				t.Errorf("Validate(%v): error %v, wantErr %t", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

func TestValidateEnumConst(t *testing.T) {
	t.Parallel()
	runValidationCases(t, []validationCase{
		{name: "enum ok", schemaJSON: `{"enum": ["a", "b"]}`, instanceJSON: `"a"`},
		{name: "enum fail", schemaJSON: `{"enum": ["a", "b"]}`, instanceJSON: `"c"`, wantErr: true, wantErrContains: `no "enum" value matched`},
		{name: "enum number ok", schemaJSON: `{"enum": [1, 2.5]}`, instanceJSON: `2.5`},
		{name: "enum object ok", schemaJSON: `{"enum": [{"a": 1}]}`, instanceJSON: `{"a": 1}`},
		{name: "const ok", schemaJSON: `{"const": 5}`, instanceJSON: `5`},
		{name: "const fail", schemaJSON: `{"const": 5}`, instanceJSON: `6`, wantErr: true},
		{name: "const object ok", schemaJSON: `{"const": {"a": [1, 2]}}`, instanceJSON: `{"a": [1, 2]}`},
	})
}

func TestValidateNumericKeywords(t *testing.T) {
	t.Parallel()
	runValidationCases(t, []validationCase{
		{name: "maximum ok", schemaJSON: `{"maximum": 5}`, instanceJSON: `5`},
		{name: "maximum fail", schemaJSON: `{"maximum": 5}`, instanceJSON: `6`, wantErr: true, wantErrContains: `larger than "maximum"`},
		{name: "maximum ignores string", schemaJSON: `{"maximum": 5}`, instanceJSON: `"6"`},
		{name: "minimum ok", schemaJSON: `{"minimum": 5}`, instanceJSON: `5`},
		{name: "minimum fail", schemaJSON: `{"minimum": 5}`, instanceJSON: `4`, wantErr: true, wantErrContains: `smaller than "minimum"`},
		{name: "minimum ignores string", schemaJSON: `{"minimum": 5}`, instanceJSON: `"4"`},
		{name: "exclusiveMaximum ok", schemaJSON: `{"exclusiveMaximum": 5}`, instanceJSON: `4.9`},
		{name: "exclusiveMaximum boundary fail", schemaJSON: `{"exclusiveMaximum": 5}`, instanceJSON: `5`, wantErr: true, wantErrContains: `not less than "exclusiveMaximum"`},
		{name: "exclusiveMinimum ok", schemaJSON: `{"exclusiveMinimum": 5}`, instanceJSON: `5.1`},
		{name: "exclusiveMinimum boundary fail", schemaJSON: `{"exclusiveMinimum": 5}`, instanceJSON: `5`, wantErr: true, wantErrContains: `not greater than "exclusiveMinimum"`},
		{name: "multipleOf ok", schemaJSON: `{"multipleOf": 1.5}`, instanceJSON: `4.5`},
		{name: "multipleOf fail", schemaJSON: `{"multipleOf": 2}`, instanceJSON: `7`, wantErr: true},
		{name: "multipleOf decimal ok", schemaJSON: `{"multipleOf": 0.01}`, instanceJSON: `19.99`},
		{name: "multipleOf small decimal ok", schemaJSON: `{"multipleOf": 0.0001}`, instanceJSON: `0.0075`},
		{name: "multipleOf decimal fail", schemaJSON: `{"multipleOf": 0.0001}`, instanceJSON: `0.00751`, wantErr: true},
		{name: "multipleOf ignores non-number", schemaJSON: `{"multipleOf": 2}`, instanceJSON: `"7"`},
		{name: "multipleOf zero is schema error", schemaJSON: `{"multipleOf": 0}`, instanceJSON: `7`, wantErr: true, wantErrContains: "must be greater than zero"},
	})
}

func TestValidateStringKeywords(t *testing.T) {
	t.Parallel()
	runValidationCases(t, []validationCase{
		{name: "maxLength ok", schemaJSON: `{"maxLength": 3}`, instanceJSON: `"abc"`},
		{name: "maxLength fail", schemaJSON: `{"maxLength": 3}`, instanceJSON: `"abcd"`, wantErr: true},
		{name: "maxLength counts runes", schemaJSON: `{"maxLength": 3}`, instanceJSON: `"åäö"`},
		{name: "minLength ok", schemaJSON: `{"minLength": 3}`, instanceJSON: `"abc"`},
		{name: "minLength fail", schemaJSON: `{"minLength": 3}`, instanceJSON: `"ab"`, wantErr: true, wantErrContains: `"minLength"`},
		{name: "minLength negative is schema error", schemaJSON: `{"minLength": -1}`, instanceJSON: `"a"`, wantErr: true, wantErrContains: `"minLength" argument is -1`},
		{name: "maxLength negative is schema error", schemaJSON: `{"maxLength": -1}`, instanceJSON: `"a"`, wantErr: true, wantErrContains: `"maxLength" argument is -1`},
		{name: "pattern ok", schemaJSON: `{"pattern": "^a+b$"}`, instanceJSON: `"aab"`},
		{name: "pattern fail", schemaJSON: `{"pattern": "^a+b$"}`, instanceJSON: `"ba"`, wantErr: true},
		{name: "pattern ignores non-string", schemaJSON: `{"pattern": "^a+b$"}`, instanceJSON: `5`},
		{name: "pattern invalid regexp is schema error", schemaJSON: `{"pattern": "("}`, instanceJSON: `"a"`, wantErr: true},
	})
}

func TestValidateArrayKeywords(t *testing.T) {
	t.Parallel()
	runValidationCases(t, []validationCase{
		{name: "items ok", schemaJSON: `{"items": {"type": "string"}}`, instanceJSON: `["a", "b"]`},
		{name: "items fail", schemaJSON: `{"items": {"type": "string"}}`, instanceJSON: `["a", 5]`, wantErr: true, wantInstanceLocation: "#/1"},
		{name: "prefixItems ok", schemaJSON: `{"prefixItems": [{"type": "string"}, {"type": "integer"}]}`, instanceJSON: `["a", 5]`},
		{name: "prefixItems fail", schemaJSON: `{"prefixItems": [{"type": "string"}, {"type": "integer"}]}`, instanceJSON: `[5, "a"]`, wantErr: true, wantInstanceLocation: "#/0"},
		{name: "prefixItems shorter instance ok", schemaJSON: `{"prefixItems": [{"type": "string"}, {"type": "integer"}]}`, instanceJSON: `["a"]`},
		{name: "prefixItems then items", schemaJSON: `{"prefixItems": [{"type": "string"}], "items": {"type": "integer"}}`, instanceJSON: `["a", 1, 2]`},
		{name: "prefixItems then items fail", schemaJSON: `{"prefixItems": [{"type": "string"}], "items": {"type": "integer"}}`, instanceJSON: `["a", 1, "b"]`, wantErr: true, wantInstanceLocation: "#/2"},
		{name: "maxItems ok", schemaJSON: `{"maxItems": 2}`, instanceJSON: `[1, 2]`},
		{name: "maxItems fail", schemaJSON: `{"maxItems": 2}`, instanceJSON: `[1, 2, 3]`, wantErr: true},
		{name: "minItems ok", schemaJSON: `{"minItems": 2}`, instanceJSON: `[1, 2]`},
		{name: "minItems fail", schemaJSON: `{"minItems": 2}`, instanceJSON: `[1]`, wantErr: true},
		{name: "uniqueItems ok", schemaJSON: `{"uniqueItems": true}`, instanceJSON: `[1, 2, 3]`},
		{name: "uniqueItems fail", schemaJSON: `{"uniqueItems": true}`, instanceJSON: `[1, 2, 1]`, wantErr: true},
		{name: "uniqueItems false ok", schemaJSON: `{"uniqueItems": false}`, instanceJSON: `[1, 1]`},
		{name: "uniqueItems non-comparable fail", schemaJSON: `{"uniqueItems": true}`, instanceJSON: `[[1], [1]]`, wantErr: true},
		{name: "uniqueItems non-comparable ok", schemaJSON: `{"uniqueItems": true}`, instanceJSON: `[[1], [2]]`},
		{name: "contains ok", schemaJSON: `{"contains": {"type": "string"}}`, instanceJSON: `[1, "a"]`},
		{name: "contains fail", schemaJSON: `{"contains": {"type": "string"}}`, instanceJSON: `[1, 2]`, wantErr: true, wantErrContains: `no array element matches "contains"`},
		{name: "minContains zero makes contains optional", schemaJSON: `{"contains": {"type": "string"}, "minContains": 0}`, instanceJSON: `[1, 2]`},
		{name: "minContains fail", schemaJSON: `{"contains": {"type": "string"}, "minContains": 2}`, instanceJSON: `["a", 1]`, wantErr: true, wantErrContains: `"minContains"`},
		{name: "maxContains ok", schemaJSON: `{"contains": {"type": "string"}, "maxContains": 2}`, instanceJSON: `["a", "b", 1]`},
		{name: "maxContains fail", schemaJSON: `{"contains": {"type": "string"}, "maxContains": 1}`, instanceJSON: `["a", "b"]`, wantErr: true, wantErrContains: `"maxContains"`},
		{name: "unevaluatedItems ok", schemaJSON: `{"prefixItems": [{"type": "string"}], "unevaluatedItems": {"type": "integer"}}`, instanceJSON: `["a", 1]`},
		{name: "unevaluatedItems fail", schemaJSON: `{"prefixItems": [{"type": "string"}], "unevaluatedItems": {"type": "integer"}}`, instanceJSON: `["a", "b"]`, wantErr: true, wantInstanceLocation: "#/1"},
		{name: "unevaluatedItems skips items", schemaJSON: `{"items": {"type": "string"}, "unevaluatedItems": {"type": "integer"}}`, instanceJSON: `["a", "b"]`},
	})
}

func TestValidateObjectKeywords(t *testing.T) {
	t.Parallel()
	runValidationCases(t, []validationCase{
		{name: "properties ok", schemaJSON: `{"properties": {"a": {"type": "integer"}}}`, instanceJSON: `{"a": 1}`},
		{name: "properties fail", schemaJSON: `{"properties": {"a": {"type": "integer"}}}`, instanceJSON: `{"a": "x"}`, wantErr: true, wantInstanceLocation: "#/a"},
		{name: "properties missing ok", schemaJSON: `{"properties": {"a": {"type": "integer"}}}`, instanceJSON: `{}`},
		{name: "nested properties location", schemaJSON: `{"properties": {"a": {"properties": {"b": {"type": "integer"}}}}}`, instanceJSON: `{"a": {"b": "x"}}`, wantErr: true, wantInstanceLocation: "#/a/b"},
		{name: "required ok", schemaJSON: `{"required": ["a"]}`, instanceJSON: `{"a": 1}`},
		{name: "required fail", schemaJSON: `{"required": ["a", "b"]}`, instanceJSON: `{"a": 1}`, wantErr: true, wantErrContains: `missing required property "b"`},
		{name: "additionalProperties false ok", schemaJSON: `{"properties": {"a": {}}, "additionalProperties": false}`, instanceJSON: `{"a": 1}`},
		{name: "additionalProperties false fail", schemaJSON: `{"properties": {"a": {}}, "additionalProperties": false}`, instanceJSON: `{"a": 1, "b": 2}`, wantErr: true, wantErrContains: `unknown property "b"`},
		{name: "additionalProperties schema ok", schemaJSON: `{"additionalProperties": {"type": "string"}}`, instanceJSON: `{"a": "x"}`},
		{name: "additionalProperties schema fail keeps message", schemaJSON: `{"additionalProperties": {"type": "string"}}`, instanceJSON: `{"a": 5}`, wantErr: true, wantErrContains: `instance has type "integer", want "string"`},
		{name: "patternProperties ok", schemaJSON: `{"patternProperties": {"^x_": {"type": "integer"}}}`, instanceJSON: `{"x_a": 1, "other": "s"}`},
		{name: "patternProperties fail", schemaJSON: `{"patternProperties": {"^x_": {"type": "integer"}}}`, instanceJSON: `{"x_a": "s"}`, wantErr: true, wantInstanceLocation: "#/x_a"},
		{name: "patternProperties with additionalProperties", schemaJSON: `{"patternProperties": {"^x_": {}}, "additionalProperties": false}`, instanceJSON: `{"x_a": 1}`},
		{name: "propertyNames ok", schemaJSON: `{"propertyNames": {"maxLength": 3}}`, instanceJSON: `{"ab": 1}`},
		{name: "propertyNames fail", schemaJSON: `{"propertyNames": {"maxLength": 3}}`, instanceJSON: `{"abcd": 1}`, wantErr: true},
		{name: "maxProperties ok", schemaJSON: `{"maxProperties": 1}`, instanceJSON: `{"a": 1}`},
		{name: "maxProperties fail", schemaJSON: `{"maxProperties": 1}`, instanceJSON: `{"a": 1, "b": 2}`, wantErr: true},
		{name: "minProperties fail", schemaJSON: `{"minProperties": 2}`, instanceJSON: `{"a": 1}`, wantErr: true},
		{name: "dependentRequired ok", schemaJSON: `{"dependentRequired": {"a": ["b"]}}`, instanceJSON: `{"a": 1, "b": 2}`},
		{name: "dependentRequired fail", schemaJSON: `{"dependentRequired": {"a": ["b"]}}`, instanceJSON: `{"a": 1}`, wantErr: true, wantErrContains: `"dependentRequired"`},
		{name: "dependentRequired absent trigger ok", schemaJSON: `{"dependentRequired": {"a": ["b"]}}`, instanceJSON: `{"c": 1}`},
		{name: "dependentSchemas ok", schemaJSON: `{"dependentSchemas": {"a": {"required": ["b"]}}}`, instanceJSON: `{"a": 1, "b": 2}`},
		{name: "dependentSchemas fail", schemaJSON: `{"dependentSchemas": {"a": {"required": ["b"]}}}`, instanceJSON: `{"a": 1}`, wantErr: true},
		{name: "dependencies array ok", schemaJSON: `{"dependencies": {"a": ["b"]}}`, instanceJSON: `{"a": 1, "b": 2}`},
		{name: "dependencies array fail", schemaJSON: `{"dependencies": {"a": ["b"]}}`, instanceJSON: `{"a": 1}`, wantErr: true},
		{name: "dependencies schema fail", schemaJSON: `{"dependencies": {"a": {"required": ["b"]}}}`, instanceJSON: `{"a": 1}`, wantErr: true},
		{name: "unevaluatedProperties ok", schemaJSON: `{"properties": {"a": {}}, "unevaluatedProperties": false}`, instanceJSON: `{"a": 1}`},
		{name: "unevaluatedProperties fail", schemaJSON: `{"properties": {"a": {}}, "unevaluatedProperties": false}`, instanceJSON: `{"a": 1, "b": 2}`, wantErr: true},
		{name: "unevaluatedProperties sees allOf", schemaJSON: `{"allOf": [{"properties": {"a": {}}}], "unevaluatedProperties": false}`, instanceJSON: `{"a": 1}`},
	})
}

func TestValidateCombinators(t *testing.T) {
	t.Parallel()
	runValidationCases(t, []validationCase{
		{name: "allOf ok", schemaJSON: `{"allOf": [{"type": "integer"}, {"minimum": 3}]}`, instanceJSON: `5`},
		{name: "allOf fail", schemaJSON: `{"allOf": [{"type": "integer"}, {"minimum": 3}]}`, instanceJSON: `2`, wantErr: true},
		{name: "anyOf first ok", schemaJSON: `{"anyOf": [{"type": "integer"}, {"type": "string"}]}`, instanceJSON: `5`},
		{name: "anyOf second ok", schemaJSON: `{"anyOf": [{"type": "integer"}, {"type": "string"}]}`, instanceJSON: `"x"`},
		{name: "anyOf fail", schemaJSON: `{"anyOf": [{"type": "integer"}, {"type": "string"}]}`, instanceJSON: `true`, wantErr: true, wantErrContains: `no "anyof" schema matches`},
		{name: "oneOf ok", schemaJSON: `{"oneOf": [{"type": "integer"}, {"type": "string"}]}`, instanceJSON: `5`},
		{name: "oneOf zero matches fail", schemaJSON: `{"oneOf": [{"type": "integer"}, {"type": "string"}]}`, instanceJSON: `true`, wantErr: true, wantErrContains: `no match for "oneof"`},
		{name: "oneOf two matches fail", schemaJSON: `{"oneOf": [{"type": "integer"}, {"minimum": 0}]}`, instanceJSON: `5`, wantErr: true, wantErrContains: `2 matches for "oneof"`},
		{name: "not ok", schemaJSON: `{"not": {"type": "string"}}`, instanceJSON: `5`},
		{name: "not fail", schemaJSON: `{"not": {"type": "string"}}`, instanceJSON: `"x"`, wantErr: true, wantErrContains: `"not" schema matched`},
		{name: "if then ok", schemaJSON: `{"if": {"type": "string"}, "then": {"minLength": 2}}`, instanceJSON: `"ab"`},
		{name: "if then fail", schemaJSON: `{"if": {"type": "string"}, "then": {"minLength": 2}}`, instanceJSON: `"a"`, wantErr: true},
		{name: "if false then skipped", schemaJSON: `{"if": {"type": "string"}, "then": {"minLength": 2}}`, instanceJSON: `5`},
		{name: "if false else applies ok", schemaJSON: `{"if": {"type": "string"}, "else": {"type": "number"}}`, instanceJSON: `5`},
		{name: "if false else applies fail", schemaJSON: `{"if": {"type": "string"}, "else": {"type": "number"}}`, instanceJSON: `true`, wantErr: true},
		{name: "if true else skipped", schemaJSON: `{"if": {"type": "string"}, "else": {"type": "number"}}`, instanceJSON: `"x"`},
		{name: "true schema", schemaJSON: `true`, instanceJSON: `{"anything": 1}`},
		{name: "false schema", schemaJSON: `false`, instanceJSON: `5`, wantErr: true, wantErrContains: "false schema never matches"},
	})
}

func TestValidateRefs(t *testing.T) {
	t.Parallel()
	runValidationCases(t, []validationCase{
		{
			name:         "ref to defs ok",
			schemaJSON:   `{"$defs": {"positive": {"type": "integer", "minimum": 1}}, "properties": {"n": {"$ref": "#/$defs/positive"}}}`,
			instanceJSON: `{"n": 3}`,
		},
		{
			name:         "ref to defs fail",
			schemaJSON:   `{"$defs": {"positive": {"type": "integer", "minimum": 1}}, "properties": {"n": {"$ref": "#/$defs/positive"}}}`,
			instanceJSON: `{"n": 0}`,
			wantErr:      true,
		},
		{
			name:         "ref to anchor ok",
			schemaJSON:   `{"$defs": {"a": {"$anchor": "positive", "type": "integer", "minimum": 1}}, "properties": {"n": {"$ref": "#positive"}}}`,
			instanceJSON: `{"n": 3}`,
		},
		{
			name:         "ref to anchor fail",
			schemaJSON:   `{"$defs": {"a": {"$anchor": "positive", "type": "integer", "minimum": 1}}, "properties": {"n": {"$ref": "#positive"}}}`,
			instanceJSON: `{"n": 0}`,
			wantErr:      true,
		},
		{
			name:         "recursive ref ok",
			schemaJSON:   `{"type": "object", "properties": {"next": {"$ref": "#"}}}`,
			instanceJSON: `{"next": {"next": {}}}`,
		},
		{
			name:         "recursive ref fail",
			schemaJSON:   `{"type": "object", "properties": {"next": {"$ref": "#"}}}`,
			instanceJSON: `{"next": {"next": 5}}`,
			wantErr:      true,
		},
		{
			name:         "dynamic ref ok",
			schemaJSON:   `{"$id": "https://test.example/root", "$dynamicAnchor": "node", "type": "object", "properties": {"next": {"$dynamicRef": "#node"}}}`,
			instanceJSON: `{"next": {"next": {}}}`,
		},
		{
			name:         "dynamic ref fail",
			schemaJSON:   `{"$id": "https://test.example/root", "$dynamicAnchor": "node", "type": "object", "properties": {"next": {"$dynamicRef": "#node"}}}`,
			instanceJSON: `{"next": 5}`,
			wantErr:      true,
		},
	})
}

func TestValidateFormat(t *testing.T) {
	t.Parallel()
	runValidationCases(t, []validationCase{
		{name: "email ok", schemaJSON: `{"format": "email"}`, instanceJSON: `"user@example.com"`},
		{name: "email fail", schemaJSON: `{"format": "email"}`, instanceJSON: `"not-an-email"`, wantErr: true},
		{name: "uuid ok", schemaJSON: `{"format": "uuid"}`, instanceJSON: `"123e4567-e89b-12d3-a456-426614174000"`},
		{name: "uuid fail", schemaJSON: `{"format": "uuid"}`, instanceJSON: `"123e4567"`, wantErr: true},
		{name: "unknown format ignored", schemaJSON: `{"format": "no-such-format"}`, instanceJSON: `"whatever"`},
		{name: "format ignores non-string", schemaJSON: `{"format": "email"}`, instanceJSON: `5`},
	})
}

func TestValidateErrorType(t *testing.T) {
	t.Parallel()
	s := mustUnmarshalSchema(t, `{"type": "string"}`)

	err := s.Validate(5)
	if err == nil {
		t.Fatal("Validate(5) returned nil, want error")
	}

	validateError, ok := errors.AsType[*schema.ValidateError](err)
	if !ok {
		t.Fatalf("Validate error is %T, want *schema.ValidateError", err)
	}
	if len(validateError.Errors) != 1 {
		t.Errorf("len(Errors) = %d, want 1", len(validateError.Errors))
	}

	if !errors.Is(err, altshiftErrors.ErrValidationError) {
		t.Error("errors.Is(err, altshiftErrors.ErrValidationError) = false, want true")
	}
}

func TestValidateStructInstance(t *testing.T) {
	t.Parallel()
	type inner struct {
		Count int `json:"count"`
	}
	type outer struct {
		Name  string `json:"name"`
		Inner *inner `json:"inner,omitzero"`
	}

	schemaJSON := `{
		"type": "object",
		"properties": {
			"name": {"type": "string", "minLength": 1},
			"inner": {
				"type": "object",
				"properties": {"count": {"type": "integer", "minimum": 0}}
			}
		},
		"required": ["name"]
	}`

	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "valid struct", instance: outer{Name: "x", Inner: &inner{Count: 1}}},
		{name: "valid pointer to struct", instance: &outer{Name: "x"}},
		{name: "missing required", instance: outer{Inner: &inner{Count: 1}}, wantErr: true},
		{name: "nested failure", instance: outer{Name: "x", Inner: &inner{Count: -1}}, wantErr: true},
		// A nil pointer instance previously panicked in the field lookup;
		// it now validates as an object without fields.
		{name: "nil pointer instance does not panic", instance: (*outer)(nil)},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s := mustUnmarshalSchema(t, schemaJSON)
			err := s.Validate(testCase.instance)
			if (err != nil) != testCase.wantErr {
				t.Errorf("Validate(%+v): error %v, wantErr %t", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

func TestValidateApplyDefaults(t *testing.T) {
	t.Parallel()
	schemaJSON := `{
		"type": "object",
		"properties": {
			"a": {"type": "integer", "default": 42},
			"b": {"type": "string"}
		}
	}`

	s := mustUnmarshalSchema(t, schemaJSON)

	instance := map[string]any{"b": "x"}
	if err := s.ValidateWithOpts(instance, &schema.ValidateOpts{ApplyDefaults: true}); err != nil {
		t.Fatalf("ValidateWithOpts: %v", err)
	}
	if got := instance["a"]; got != float64(42) {
		t.Errorf(`instance["a"] = %v (%T), want 42`, got, got)
	}
}

func TestValidateDepthLimit(t *testing.T) {
	t.Parallel()
	// A schema that refers to itself for the same instance recurses
	// until the depth limit stops it with a non-validation error.
	s := mustUnmarshalSchema(t, `{"$ref": "#"}`)
	err := s.Validate(5)
	if err == nil {
		t.Fatal("Validate returned nil, want recursion error")
	}
	if !strings.Contains(err.Error(), "recursion") {
		t.Errorf("error %q does not mention recursion", err.Error())
	}
}
