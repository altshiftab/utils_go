package draft202012_test

import (
	"encoding/json/v2"
	"reflect"
	"testing"

	"github.com/altshiftab/utils_go/pkg/json/schema/draft202012"
	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

func TestBuilderKeywords(t *testing.T) {
	t.Parallel()
	sub := func(typ string) *schema.Schema {
		return draft202012.NewSubBuilder().AddType(typ).Build()
	}

	testCases := []struct {
		name     string
		build    func() *schema.Schema
		wantJSON string
	}{
		{
			name: "combinators",
			build: func() *schema.Schema {
				return draft202012.NewSubBuilder().
					AddAllOf([]*schema.Schema{sub("integer")}).
					AddAnyOf([]*schema.Schema{sub("integer"), sub("string")}).
					AddOneOf([]*schema.Schema{sub("integer"), sub("string")}).
					AddNot(sub("null")).
					Build()
			},
			wantJSON: `{
				"allOf": [{"type": "integer"}],
				"anyOf": [{"type": "integer"}, {"type": "string"}],
				"oneOf": [{"type": "integer"}, {"type": "string"}],
				"not": {"type": "null"}
			}`,
		},
		{
			name: "conditionals",
			build: func() *schema.Schema {
				return draft202012.NewSubBuilder().
					AddIf(sub("string")).
					AddThen(draft202012.NewSubBuilder().AddMinLength(1).Build()).
					AddElse(sub("integer")).
					Build()
			},
			wantJSON: `{
				"if": {"type": "string"},
				"then": {"minLength": 1},
				"else": {"type": "integer"}
			}`,
		},
		{
			name: "array keywords",
			build: func() *schema.Schema {
				return draft202012.NewSubBuilder().
					AddType("array").
					AddPrefixItems([]*schema.Schema{sub("string")}).
					AddItems(sub("integer")).
					AddContains(sub("integer")).
					AddMinContains(1).
					AddMaxContains(5).
					AddMinItems(1).
					AddMaxItems(10).
					AddUniqueItems(true).
					AddUnevaluatedItems(draft202012.NewSubBuilder().Build()).
					Build()
			},
			wantJSON: `{
				"type": "array",
				"prefixItems": [{"type": "string"}],
				"items": {"type": "integer"},
				"contains": {"type": "integer"},
				"minContains": 1,
				"maxContains": 5,
				"minItems": 1,
				"maxItems": 10,
				"uniqueItems": true,
				"unevaluatedItems": {}
			}`,
		},
		{
			name: "object keywords",
			build: func() *schema.Schema {
				return draft202012.NewSubBuilder().
					AddType("object").
					AddProperties(map[string]*schema.Schema{"a": sub("integer")}).
					AddPatternProperties(map[string]*schema.Schema{"^x_": sub("string")}).
					AddAdditionalProperties(sub("boolean")).
					AddPropertyNames(draft202012.NewSubBuilder().AddMaxLength(5).Build()).
					AddRequired([]string{"a"}).
					AddDependentRequired(map[string]any{"a": []any{"b"}}).
					AddDependentSchemas(map[string]*schema.Schema{"a": draft202012.NewSubBuilder().Build()}).
					AddDependencies(map[string]schema.ArrayOrSchema{"c": {Array: []string{"d"}}}).
					AddMinProperties(1).
					AddMaxProperties(10).
					AddUnevaluatedProperties(draft202012.NewSubBuilder().Build()).
					Build()
			},
			wantJSON: `{
				"type": "object",
				"properties": {"a": {"type": "integer"}},
				"patternProperties": {"^x_": {"type": "string"}},
				"additionalProperties": {"type": "boolean"},
				"propertyNames": {"maxLength": 5},
				"required": ["a"],
				"dependentRequired": {"a": ["b"]},
				"dependentSchemas": {"a": {}},
				"dependencies": {"c": ["d"]},
				"minProperties": 1,
				"maxProperties": 10,
				"unevaluatedProperties": {}
			}`,
		},
		{
			name: "annotations and content",
			build: func() *schema.Schema {
				return draft202012.NewSubBuilder().
					AddComment("a comment").
					AddTitle("a title").
					AddDescription("a description").
					AddDefault(5).
					AddDeprecated(true).
					AddReadOnly(true).
					AddWriteOnly(false).
					AddExamples([]any{1, 2}).
					AddFormat("email").
					AddContentEncoding("base64").
					AddContentMediaType("application/json").
					AddContentSchema(sub("object")).
					AddEnum([]any{"a", "b"}).
					AddConst("a").
					AddMultipleOf(2).
					AddExclusiveMinimum(0).
					AddExclusiveMaximum(100).
					Build()
			},
			wantJSON: `{
				"$comment": "a comment",
				"title": "a title",
				"description": "a description",
				"default": 5,
				"deprecated": true,
				"readOnly": true,
				"writeOnly": false,
				"examples": [1, 2],
				"format": "email",
				"contentEncoding": "base64",
				"contentMediaType": "application/json",
				"contentSchema": {"type": "object"},
				"enum": ["a", "b"],
				"const": "a",
				"multipleOf": 2,
				"exclusiveMinimum": 0,
				"exclusiveMaximum": 100
			}`,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			data, err := json.Marshal(testCase.build())
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var got, want any
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("marshal output %q is not valid JSON: %v", data, err)
			}
			if err := json.Unmarshal([]byte(testCase.wantJSON), &want); err != nil {
				t.Fatalf("unmarshal want: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("built schema mismatch:\ngot  %s\nwant %s", data, testCase.wantJSON)
			}
		})
	}
}

func TestBuilderBoolSchema(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		acceptAll bool
		wantErr   bool
	}{
		{name: "true schema accepts", acceptAll: true},
		{name: "false schema rejects", acceptAll: false, wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s := draft202012.NewBuilder().BoolSchema(testCase.acceptAll)
			err := s.Validate(map[string]any{"anything": 1})
			if (err != nil) != testCase.wantErr {
				t.Errorf("Validate: error %v, wantErr %t", err, testCase.wantErr)
			}
		})
	}
}

func TestNewBuilderValidates(t *testing.T) {
	t.Parallel()
	s := draft202012.NewBuilder().
		AddType("object").
		AddProperties(map[string]*schema.Schema{
			"name": draft202012.NewSubBuilder().AddType("string").AddMinLength(1).Build(),
		}).
		AddRequired([]string{"name"}).
		Build()

	if err := s.Validate(map[string]any{"name": "x"}); err != nil {
		t.Errorf("Validate(valid): %v", err)
	}
	if err := s.Validate(map[string]any{}); err == nil {
		t.Error("Validate(missing required) succeeded, want error")
	}
}

func TestValidateAgainstMetaSchema(t *testing.T) {
	t.Parallel()
	// A $ref to the draft 2020-12 meta-schema URI loads the
	// embedded meta-schema.
	metaRef := mustUnmarshalSchema(t, `{"$ref": "https://json-schema.org/draft/2020-12/schema"}`)

	testCases := []struct {
		name         string
		instanceJSON string
		wantErr      bool
	}{
		{name: "valid schema document", instanceJSON: `{"type": "string", "minLength": 1}`},
		{name: "invalid schema document", instanceJSON: `{"type": 5}`, wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := metaRef.Validate(mustUnmarshalInstance(t, testCase.instanceJSON))
			if (err != nil) != testCase.wantErr {
				t.Errorf("Validate(%s): error %v, wantErr %t", testCase.instanceJSON, err, testCase.wantErr)
			}
		})
	}
}

func TestValidateApplyDefaultsStruct(t *testing.T) {
	t.Parallel()
	type config struct {
		Level string `json:"level"`
		Port  int    `json:"port"`
	}

	schemaJSON := `{
		"type": "object",
		"properties": {
			"level": {"type": "string", "default": "info"},
			"port": {"type": "integer", "default": 8080},
			"unset": {"type": "string", "default": "x"}
		}
	}`
	s := mustUnmarshalSchema(t, schemaJSON)

	instance := &config{}
	if err := s.ValidateWithOpts(instance, &schema.ValidateOpts{ApplyDefaults: true}); err != nil {
		t.Fatalf("ValidateWithOpts: %v", err)
	}
	if instance.Level != "info" {
		t.Errorf("Level = %q, want %q", instance.Level, "info")
	}
	if instance.Port != 8080 {
		t.Errorf("Port = %d, want 8080", instance.Port)
	}
}

func TestValidateApplyDefaultsPrefixItems(t *testing.T) {
	t.Parallel()
	schemaJSON := `{
		"type": "array",
		"prefixItems": [
			{"type": "string", "default": "first"},
			{"type": "integer"}
		]
	}`
	s := mustUnmarshalSchema(t, schemaJSON)

	instance := []any{"", float64(2)}
	if err := s.ValidateWithOpts(instance, &schema.ValidateOpts{ApplyDefaults: true}); err != nil {
		t.Fatalf("ValidateWithOpts: %v", err)
	}
	if instance[0] != "first" {
		t.Errorf("instance[0] = %v, want %q", instance[0], "first")
	}
}
