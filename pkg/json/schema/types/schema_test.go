package types_test

import (
	"encoding/json/v2"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/json/schema/draft202012"
	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

// mustUnmarshal parses schemaJSON into a resolved schema.
func mustUnmarshal(t *testing.T, schemaJSON string) *schema.Schema {
	t.Helper()
	var s schema.Schema
	if err := json.Unmarshal([]byte(schemaJSON), &s); err != nil {
		t.Fatalf("unmarshal schema %s: %v", schemaJSON, err)
	}
	return &s
}

func TestMarshalRoundTrip(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name       string
		schemaJSON string
	}{
		{name: "empty object", schemaJSON: `{}`},
		{name: "true schema", schemaJSON: `true`},
		{name: "false schema", schemaJSON: `false`},
		{name: "scalar keywords", schemaJSON: `{"type": "string", "minLength": 1, "maxLength": 10, "pattern": "^a"}`},
		{name: "numeric keywords", schemaJSON: `{"minimum": 1.5, "maximum": 3, "multipleOf": 0.5}`},
		{name: "type array", schemaJSON: `{"type": ["string", "null"]}`},
		{name: "nested schemas", schemaJSON: `{"properties": {"a": {"type": "integer"}, "b": {"items": {"type": "string"}}}, "required": ["a"]}`},
		{name: "schema arrays", schemaJSON: `{"allOf": [{"minimum": 0}, {"maximum": 10}], "prefixItems": [{"type": "string"}]}`},
		{name: "bool subschema", schemaJSON: `{"additionalProperties": false}`},
		{name: "enum and const", schemaJSON: `{"enum": ["a", 1, null], "const": {"k": [1, 2]}}`},
		{name: "unknown keyword", schemaJSON: `{"x-custom": {"nested": [1, "two"]}}`},
		{name: "dependencies mixed", schemaJSON: `{"dependencies": {"a": ["b"], "c": {"type": "object"}}}`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s := mustUnmarshal(t, testCase.schemaJSON)

			data, err := json.Marshal(s)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			// The unmarshaler adds "$schema" when missing;
			// compare against the input with that keyword added.
			var want any
			if err := json.Unmarshal([]byte(testCase.schemaJSON), &want); err != nil {
				t.Fatalf("unmarshal want: %v", err)
			}
			if m, ok := want.(map[string]any); ok {
				if _, ok := m["$schema"]; !ok {
					m["$schema"] = draft202012.SchemaID
				}
			}

			var got any
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("marshal output %q is not valid JSON: %v", data, err)
			}

			if !reflect.DeepEqual(got, want) {
				t.Errorf("round trip mismatch:\ngot  %s\nwant %v", data, want)
			}
		})
	}
}

func TestMarshalSchemaOrSchemas(t *testing.T) {
	t.Parallel()
	subSchema := func(typ string) *schema.Schema {
		return draft202012.NewSubBuilder().AddType(typ).Build()
	}

	keyword := &schema.Keyword{
		Name:    "custom",
		ArgType: schema.ArgTypeSchemaOrSchemas,
	}

	testCases := []struct {
		name  string
		value schema.PartSchemaOrSchemas
		want  string
	}{
		{
			name:  "single schema",
			value: schema.PartSchemaOrSchemas{Schema: subSchema("string")},
			want:  `{"custom":{"type":"string"}}`,
		},
		{
			name: "schema array has comma separators",
			value: schema.PartSchemaOrSchemas{
				Schemas: []*schema.Schema{subSchema("string"), subSchema("integer"), subSchema("null")},
			},
			want: `{"custom":[{"type":"string"},{"type":"integer"},{"type":"null"}]}`,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s := &schema.Schema{Parts: []schema.Part{schema.MakePart(keyword, testCase.value)}}
			data, err := json.Marshal(s)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(data) != testCase.want {
				t.Errorf("marshal = %s, want %s", data, testCase.want)
			}
		})
	}
}

func TestMarshalPartAnyNoTrailingNewline(t *testing.T) {
	t.Parallel()
	s := mustUnmarshal(t, `{"x-custom": [1, 2], "type": "string"}`)
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "\n") {
		t.Errorf("marshal output contains newline: %q", data)
	}
}

func TestMarshalFloatFormatting(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name  string
		value float64
		want  string
	}{
		{name: "integral value", value: 3, want: `"maximum":3`},
		{name: "fractional value", value: 3.5, want: `"maximum":3.5`},
		{name: "large value", value: 1e18, want: `"maximum":1000000000000000000`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s := draft202012.NewSubBuilder().AddMaximum(testCase.value).Build()
			data, err := json.Marshal(s)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if !strings.Contains(string(data), testCase.want) {
				t.Errorf("marshal = %s, want it to contain %s", data, testCase.want)
			}
		})
	}
}

func TestMarshalNonFiniteFloat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name  string
		value float64
	}{
		{name: "NaN", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s := draft202012.NewSubBuilder().AddMaximum(testCase.value).Build()
			if _, err := json.Marshal(s); err == nil {
				t.Error("marshal of non-finite float succeeded, want error")
			}
		})
	}
}

func TestLookupKeyword(t *testing.T) {
	t.Parallel()
	s := mustUnmarshal(t, `{"type": "string", "minLength": 2}`)

	testCases := []struct {
		name      string
		keyword   string
		wantFound bool
	}{
		{name: "present", keyword: "minLength", wantFound: true},
		{name: "absent", keyword: "maxLength", wantFound: false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			pv, found := s.LookupKeyword(testCase.keyword)
			if found != testCase.wantFound {
				t.Fatalf("LookupKeyword(%q) found = %t, want %t", testCase.keyword, found, testCase.wantFound)
			}
			if found {
				if got := pv.(schema.PartInt); got != 2 {
					t.Errorf("LookupKeyword(%q) = %v, want 2", testCase.keyword, got)
				}
			}
		})
	}
}

func TestChildren(t *testing.T) {
	t.Parallel()
	s := mustUnmarshal(t, `{
		"properties": {"b": {}, "a": {}},
		"items": {"type": "string"},
		"allOf": [{}, {}]
	}`)

	var names []string
	for name := range s.Children() {
		names = append(names, name)
	}

	// Children iterates in keyword sort order; map-valued keywords
	// iterate their keys in sorted order.
	want := []string{"allOf/0", "allOf/1", "items", "properties/a", "properties/b"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("Children names = %v, want %v", names, want)
	}
}

func TestClone(t *testing.T) {
	t.Parallel()
	s := mustUnmarshal(t, `{"type": "string"}`)
	clone := s.Clone()

	if !reflect.DeepEqual(s.Parts, clone.Parts) {
		t.Error("clone parts differ from original")
	}

	clone.Parts = clone.Parts[:0]
	if len(s.Parts) == 0 {
		t.Error("truncating the clone affected the original")
	}
}

func TestUnmarshalErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name       string
		schemaJSON string
	}{
		{name: "invalid JSON", schemaJSON: `{`},
		{name: "duplicate keyword", schemaJSON: `{"minLength": 1, "minLength": 2}`},
		{name: "unknown schema version", schemaJSON: `{"$schema": "https://example.com/no-such-draft"}`},
		{name: "non-string schema version", schemaJSON: `{"$schema": 5}`},
		{name: "wrong argument type bool", schemaJSON: `{"uniqueItems": "yes"}`},
		{name: "wrong argument type string", schemaJSON: `{"pattern": 5}`},
		{name: "wrong argument type int", schemaJSON: `{"minLength": 1.5}`},
		{name: "wrong argument type strings", schemaJSON: `{"required": [1]}`},
		{name: "wrong argument type map", schemaJSON: `{"properties": []}`},
		{name: "invalid subschema type", schemaJSON: `{"items": 5}`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var s schema.Schema
			if err := json.Unmarshal([]byte(testCase.schemaJSON), &s); err == nil {
				t.Errorf("unmarshal of %s succeeded, want error", testCase.schemaJSON)
			}
		})
	}
}

func TestLookupVocabulary(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		schemaID  string
		wantFound bool
	}{
		{name: "exact", schemaID: draft202012.SchemaID, wantFound: true},
		{name: "trailing hash", schemaID: draft202012.SchemaID + "#", wantFound: true},
		{name: "unknown", schemaID: "https://example.com/nope", wantFound: false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			v := schema.LookupVocabulary(testCase.schemaID)
			if (v != nil) != testCase.wantFound {
				t.Errorf("LookupVocabulary(%q) = %v, wantFound %t", testCase.schemaID, v, testCase.wantFound)
			}
		})
	}
}

func TestInstancePointer(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name   string
		tokens []string
		want   string
	}{
		{name: "empty", tokens: nil, want: "#"},
		{name: "simple", tokens: []string{"a", "b"}, want: "#/a/b"},
		{name: "escapes slash", tokens: []string{"a/b"}, want: "#/a~1b"},
		{name: "escapes tilde", tokens: []string{"a~b"}, want: "#/a~0b"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var vs schema.ValidationState
			for _, token := range testCase.tokens {
				vs.PushInstanceToken(token)
			}
			if got := vs.InstancePointer(); got != testCase.want {
				t.Errorf("InstancePointer() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestSchemaFromJSONKeepsInput(t *testing.T) {
	t.Parallel()
	input := map[string]any{
		"$schema": draft202012.SchemaID,
		"type":    "integer",
	}

	if _, err := schema.SchemaFromJSON("", nil, input); err != nil {
		t.Fatalf("SchemaFromJSON: %v", err)
	}

	if _, ok := input["$schema"]; !ok {
		t.Error(`SchemaFromJSON removed "$schema" from the input map`)
	}
}

func TestSchemaFromJSON(t *testing.T) {
	t.Parallel()
	var v any
	if err := json.Unmarshal([]byte(`{"type": "integer"}`), &v); err != nil {
		t.Fatal(err)
	}

	s, err := schema.SchemaFromJSON(draft202012.SchemaID, nil, v)
	if err != nil {
		t.Fatalf("SchemaFromJSON: %v", err)
	}
	if err := s.Resolve(nil); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if err := s.Validate(5); err != nil {
		t.Errorf("Validate(5): %v", err)
	}
	if err := s.Validate("x"); err == nil {
		t.Error(`Validate("x") succeeded, want error`)
	}
}

func TestString(t *testing.T) {
	t.Parallel()
	s := mustUnmarshal(t, `{"type": "string"}`)
	got := s.String()
	if !strings.Contains(got, "type") {
		t.Errorf("String() = %q, want it to mention the type keyword", got)
	}
}
