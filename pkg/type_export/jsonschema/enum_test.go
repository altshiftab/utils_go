package jsonschema

import (
	jsonv2 "encoding/json/v2"
	"errors"
	"reflect"
	"slices"
	"testing"

	typeExportErrors "github.com/altshiftab/utils_go/pkg/type_export/errors"
)

type enumRole string

func (enumRole) EnumValues() []string { return []string{"admin", "dd"} }

type namedString string

type enumNumber int

func (enumNumber) EnumValues() []string { return []string{"1"} }

type enumObject struct {
	Role         enumRole   `json:"role"`
	Roles        []enumRole `json:"roles"`
	OptionalRole *enumRole  `json:"optional_role"`
	Kind         string     `json:"kind" jsonschema:"kind,enum:BankID,enum:passport"`
	Kinds        []string   `json:"kinds" jsonschema:"kinds,enum:a,enum:b"`
	OptionalKind *string    `json:"optional_kind" jsonschema:"optional_kind,enum:x"`
}

type enumTagOnNamedString struct {
	Value namedString `json:"value" jsonschema:"value,enum:a"`
}

type enumTagOnInt struct {
	Value int `json:"value" jsonschema:"value,enum:1"`
}

type enumOnInt struct {
	Value enumNumber `json:"value"`
}

func convertProperties(t *testing.T, targetType reflect.Type) map[string]any {
	t.Helper()

	schemaData, err := Convert(targetType)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	var document map[string]any
	if err := jsonv2.Unmarshal([]byte(schemaData), &document); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	title, _ := document["title"].(string)
	definitions, _ := document["$defs"].(map[string]any)
	definition, _ := definitions[title].(map[string]any)
	properties, _ := definition["properties"].(map[string]any)
	if properties == nil {
		t.Fatalf("no properties in %s", schemaData)
	}

	return properties
}

func TestConvertEnum(t *testing.T) {
	t.Parallel()

	properties := convertProperties(t, reflect.TypeFor[enumObject]())

	testCases := []struct {
		name     string
		property string
		items    bool
		expected []any
	}{
		{name: "type-level scalar", property: "role", expected: []any{"admin", "dd"}},
		{name: "type-level slice", property: "roles", items: true, expected: []any{"admin", "dd"}},
		{name: "type-level nullable", property: "optional_role", expected: []any{"admin", "dd", nil}},
		{name: "tag-level scalar keeps case", property: "kind", expected: []any{"BankID", "passport"}},
		{name: "tag-level slice", property: "kinds", items: true, expected: []any{"a", "b"}},
		{name: "tag-level nullable", property: "optional_kind", expected: []any{"x", nil}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			schema, _ := properties[testCase.property].(map[string]any)
			if schema == nil {
				t.Fatalf("no schema for %q", testCase.property)
			}
			if testCase.items {
				schema, _ = schema["items"].(map[string]any)
				if schema == nil {
					t.Fatalf("no items schema for %q", testCase.property)
				}
			}

			enumValues, _ := schema["enum"].([]any)
			if !slices.Equal(enumValues, testCase.expected) {
				t.Errorf("enum = %v, expected %v", enumValues, testCase.expected)
			}
			if _, ok := schema["minLength"]; ok {
				t.Errorf("an enum carries minLength: %v", schema)
			}
		})
	}
}

func TestConvertEnumErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		targetType  reflect.Type
		expectedErr error
	}{
		{name: "tag on a named string", targetType: reflect.TypeFor[enumTagOnNamedString](), expectedErr: typeExportErrors.ErrEnumTagOnNamedType},
		{name: "tag on an int", targetType: reflect.TypeFor[enumTagOnInt](), expectedErr: typeExportErrors.ErrUnsupportedKind},
		{name: "enum type of int kind", targetType: reflect.TypeFor[enumOnInt](), expectedErr: typeExportErrors.ErrEnumNotString},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := Convert(testCase.targetType); !errors.Is(err, testCase.expectedErr) {
				t.Fatalf("err = %v, expected %v", err, testCase.expectedErr)
			}
		})
	}
}
