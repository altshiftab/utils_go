package types

import (
	"encoding/json/v2"
	"reflect"
	"testing"

	typeExportContext "github.com/altshiftab/utils_go/pkg/type_export/types/context"
)

type SimpleStruct struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestRenderRootStruct(t *testing.T) {
	t.Parallel()

	ctx := &Context{Context: typeExportContext.New()}
	rt := reflect.TypeFor[SimpleStruct]()
	if err := ctx.Add(rt); err != nil {
		t.Fatal(err)
	}

	output, err := ctx.RenderRoot(rt)
	if err != nil {
		t.Fatal(err)
	}

	var schema map[string]any
	if err := json.Unmarshal([]byte(output), &schema); err != nil {
		t.Fatal(err)
	}

	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Errorf("unexpected $schema: %v", schema["$schema"])
	}

	// For a struct root, it should have a $ref and no "type" key.
	if _, ok := schema["$ref"]; !ok {
		t.Error("expected $ref for struct root")
	}
	if _, ok := schema["type"]; ok {
		t.Error("struct root should not have top-level type")
	}
}

func TestRenderRootSlice(t *testing.T) {
	t.Parallel()

	ctx := &Context{Context: typeExportContext.New()}
	rt := reflect.TypeFor[[]SimpleStruct]()
	if err := ctx.Add(rt); err != nil {
		t.Fatal(err)
	}

	output, err := ctx.RenderRoot(rt)
	if err != nil {
		t.Fatal(err)
	}

	var schema map[string]any
	if err := json.Unmarshal([]byte(output), &schema); err != nil {
		t.Fatal(err)
	}

	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Errorf("unexpected $schema: %v", schema["$schema"])
	}

	// For a slice root, it should have type "array" and items with a $ref.
	if schema["type"] != "array" {
		t.Errorf("expected type array, got %v", schema["type"])
	}

	items, ok := schema["items"].(map[string]any)
	if !ok {
		t.Fatal("expected items to be a map")
	}
	ref, ok := items["$ref"].(string)
	if !ok || ref == "" {
		t.Error("expected items.$ref to be a non-empty string")
	}

	// Title should end with "Array"
	title, _ := schema["title"].(string)
	if len(title) < 6 || title[len(title)-5:] != "Array" {
		t.Errorf("expected title to end with Array, got %q", title)
	}

	// Should not have a top-level $ref (that's for struct roots)
	if _, ok := schema["$ref"]; ok {
		t.Error("array root should not have top-level $ref")
	}
}

func TestRenderRootPointerToSlice(t *testing.T) {
	t.Parallel()

	ctx := &Context{Context: typeExportContext.New()}
	rt := reflect.TypeFor[*[]SimpleStruct]()
	if err := ctx.Add(rt); err != nil {
		t.Fatal(err)
	}

	output, err := ctx.RenderRoot(rt)
	if err != nil {
		t.Fatal(err)
	}

	var schema map[string]any
	if err := json.Unmarshal([]byte(output), &schema); err != nil {
		t.Fatal(err)
	}

	// Pointer to slice should be treated the same as a slice.
	if schema["type"] != "array" {
		t.Errorf("expected type array, got %v", schema["type"])
	}
}

func TestRenderRootArray(t *testing.T) {
	t.Parallel()

	ctx := &Context{Context: typeExportContext.New()}
	rt := reflect.TypeFor[[3]SimpleStruct]()
	if err := ctx.Add(rt); err != nil {
		t.Fatal(err)
	}

	output, err := ctx.RenderRoot(rt)
	if err != nil {
		t.Fatal(err)
	}

	var schema map[string]any
	if err := json.Unmarshal([]byte(output), &schema); err != nil {
		t.Fatal(err)
	}

	if schema["type"] != "array" {
		t.Errorf("expected type array, got %v", schema["type"])
	}
}

// renderRootSchema is a small helper that registers the type, renders, and
// parses the resulting JSON for inspection.
func renderRootSchema(t *testing.T, rt reflect.Type) map[string]any {
	t.Helper()
	ctx := &Context{Context: typeExportContext.New()}
	if err := ctx.Add(rt); err != nil {
		t.Fatalf("ctx.Add: %v", err)
	}
	output, err := ctx.RenderRoot(rt)
	if err != nil {
		t.Fatalf("ctx.RenderRoot: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(output), &schema); err != nil {
		t.Fatalf("unmarshal output: %v\noutput: %s", err, output)
	}
	return schema
}

// resolveDef returns the named definition from the $defs section. Fails the
// test if the definition is missing or not an object.
func resolveDef(t *testing.T, schema map[string]any, name string) map[string]any {
	t.Helper()
	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("expected $defs to be a map, got %T", schema["$defs"])
	}
	def, ok := defs[name].(map[string]any)
	if !ok {
		t.Fatalf("expected $defs[%q] to be a map, got %T", name, defs[name])
	}
	return def
}

type Primitives struct {
	S string  `json:"s"`
	I int     `json:"i"`
	F float64 `json:"f"`
	B bool    `json:"b"`
}

func TestPrimitiveTypeMappings(t *testing.T) {
	t.Parallel()

	schema := renderRootSchema(t, reflect.TypeFor[Primitives]())
	def := resolveDef(t, schema, "Primitives")

	properties, ok := def["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map, got %T", def["properties"])
	}

	expected := map[string]string{
		"s": "string",
		"i": "integer",
		"f": "number",
		"b": "boolean",
	}
	for fieldName, wantType := range expected {
		propMap, ok := properties[fieldName].(map[string]any)
		if !ok {
			t.Errorf("missing property %q", fieldName)
			continue
		}
		if propMap["type"] != wantType {
			t.Errorf("property %q: type = %v, want %q", fieldName, propMap["type"], wantType)
		}
	}
}

type WithValidationTags struct {
	Code  string   `json:"code" jsonschema:"code,minlength:3,maxlength:8"`
	Score int      `json:"score" jsonschema:"score,minimum:0,maximum:100"`
	Tags  []string `json:"tags" jsonschema:"tags,minitems:1,maxitems:5"`
}

func TestValidationTagsAreApplied(t *testing.T) {
	t.Parallel()

	schema := renderRootSchema(t, reflect.TypeFor[WithValidationTags]())
	def := resolveDef(t, schema, "WithValidationTags")
	props := def["properties"].(map[string]any)

	code := props["code"].(map[string]any)
	if v, ok := code["minLength"]; !ok || toFloat(t, v) != 3 {
		t.Errorf("code.minLength = %v, want 3", v)
	}
	if v, ok := code["maxLength"]; !ok || toFloat(t, v) != 8 {
		t.Errorf("code.maxLength = %v, want 8", v)
	}

	score := props["score"].(map[string]any)
	if v, ok := score["minimum"]; !ok || toFloat(t, v) != 0 {
		t.Errorf("score.minimum = %v, want 0", v)
	}
	if v, ok := score["maximum"]; !ok || toFloat(t, v) != 100 {
		t.Errorf("score.maximum = %v, want 100", v)
	}

	tags := props["tags"].(map[string]any)
	if v, ok := tags["minItems"]; !ok || toFloat(t, v) != 1 {
		t.Errorf("tags.minItems = %v, want 1", v)
	}
	if v, ok := tags["maxItems"]; !ok || toFloat(t, v) != 5 {
		t.Errorf("tags.maxItems = %v, want 5", v)
	}
}

// toFloat normalizes JSON-decoded numerics (json.Unmarshal can yield int or
// float64 depending on the field's underlying type).
func toFloat(t *testing.T, v any) float64 {
	t.Helper()
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	default:
		t.Fatalf("expected numeric, got %T", v)
		return 0
	}
}

type Outer struct {
	Inner SimpleStruct `json:"inner"`
}

func TestNestedStructAddsBothToDefs(t *testing.T) {
	t.Parallel()

	schema := renderRootSchema(t, reflect.TypeFor[Outer]())

	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("expected $defs map, got %T", schema["$defs"])
	}
	if _, ok := defs["Outer"]; !ok {
		t.Errorf("$defs missing Outer: %v", keysOf(defs))
	}
	if _, ok := defs["SimpleStruct"]; !ok {
		t.Errorf("$defs missing SimpleStruct: %v", keysOf(defs))
	}

	outer := defs["Outer"].(map[string]any)
	props := outer["properties"].(map[string]any)
	innerProp, ok := props["inner"].(map[string]any)
	if !ok {
		t.Fatal("expected inner property to be an object")
	}
	if ref := innerProp["$ref"]; ref != "#/$defs/SimpleStruct" {
		t.Errorf("inner.$ref = %v, want #/$defs/SimpleStruct", ref)
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

type WithPointers struct {
	OptionalString *string       `json:"optional_string,omitempty"`
	OptionalInt    *int          `json:"optional_int,omitempty"`
	OptionalNested *SimpleStruct `json:"optional_nested,omitempty"`
}

func TestPointerFieldsAreNullable(t *testing.T) {
	t.Parallel()

	schema := renderRootSchema(t, reflect.TypeFor[WithPointers]())
	def := resolveDef(t, schema, "WithPointers")
	props := def["properties"].(map[string]any)

	stringProp := props["optional_string"].(map[string]any)
	stringTypes, ok := stringProp["type"].([]any)
	if !ok {
		t.Fatalf("optional_string.type should be a slice, got %T (%v)", stringProp["type"], stringProp["type"])
	}
	if !containsAny(stringTypes, "string") || !containsAny(stringTypes, "null") {
		t.Errorf("optional_string.type = %v, want [string null]", stringTypes)
	}

	intProp := props["optional_int"].(map[string]any)
	intTypes, ok := intProp["type"].([]any)
	if !ok {
		t.Fatalf("optional_int.type should be a slice, got %T (%v)", intProp["type"], intProp["type"])
	}
	if !containsAny(intTypes, "integer") || !containsAny(intTypes, "null") {
		t.Errorf("optional_int.type = %v, want [integer null]", intTypes)
	}

	nestedProp := props["optional_nested"].(map[string]any)
	anyOf, ok := nestedProp["anyOf"].([]any)
	if !ok {
		t.Fatalf("optional_nested should use anyOf for $ref + null, got %v", nestedProp)
	}
	sawRef, sawNull := false, false
	for _, alt := range anyOf {
		altMap, _ := alt.(map[string]any)
		if _, ok := altMap["$ref"]; ok {
			sawRef = true
		}
		if altMap["type"] == "null" {
			sawNull = true
		}
	}
	if !sawRef || !sawNull {
		t.Errorf("optional_nested.anyOf should contain both $ref and {type:null}, got %v", anyOf)
	}
}

func containsAny(haystack []any, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

type WithBytes struct {
	Blob []byte `json:"blob"`
}

func TestBytesBecomeBase64String(t *testing.T) {
	t.Parallel()

	schema := renderRootSchema(t, reflect.TypeFor[WithBytes]())
	def := resolveDef(t, schema, "WithBytes")
	blob := def["properties"].(map[string]any)["blob"].(map[string]any)

	if blob["type"] != "string" {
		t.Errorf("type = %v, want string", blob["type"])
	}
	if blob["contentEncoding"] != "base64" {
		t.Errorf("contentEncoding = %v, want base64", blob["contentEncoding"])
	}
}

type WithOptional struct {
	Required string `json:"required"`
	Maybe    string `json:"maybe,omitempty"`
	Skipped  string `json:"-"`
}

func TestOptionalAndSkippedFields(t *testing.T) {
	t.Parallel()

	schema := renderRootSchema(t, reflect.TypeFor[WithOptional]())
	def := resolveDef(t, schema, "WithOptional")

	props := def["properties"].(map[string]any)
	if _, ok := props["maybe"]; !ok {
		t.Error("expected maybe field to be present")
	}
	if _, ok := props["-"]; ok {
		t.Error("skipped (json:\"-\") field should not appear")
	}

	required, _ := def["required"].([]any)
	requiredSet := map[string]bool{}
	for _, r := range required {
		if s, ok := r.(string); ok {
			requiredSet[s] = true
		}
	}
	if !requiredSet["required"] {
		t.Errorf("`required` field should be in required list, got %v", required)
	}
	if requiredSet["maybe"] {
		t.Errorf("`maybe` (omitempty) should not be in required list, got %v", required)
	}
}

func TestMakeNullable(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    map[string]any
		expected map[string]any
	}{
		{
			name:  "ref is wrapped in anyOf",
			input: map[string]any{"$ref": "#/$defs/X"},
			expected: map[string]any{
				"anyOf": []any{
					map[string]any{"$ref": "#/$defs/X"},
					map[string]any{"type": "null"},
				},
			},
		},
		{
			name:     "plain type is widened to a tuple",
			input:    map[string]any{"type": "string"},
			expected: map[string]any{"type": []any{"string", "null"}},
		},
		{
			name:     "null type is unchanged",
			input:    map[string]any{"type": "null"},
			expected: map[string]any{"type": "null"},
		},
		{
			name:     "type tuple gets null appended",
			input:    map[string]any{"type": []any{"string", "integer"}},
			expected: map[string]any{"type": []any{"string", "integer", "null"}},
		},
		{
			name:     "type tuple with null is unchanged",
			input:    map[string]any{"type": []any{"string", "null"}},
			expected: map[string]any{"type": []any{"string", "null"}},
		},
		{
			name:  "schema without type is wrapped in anyOf",
			input: map[string]any{"enum": []any{"a", "b"}},
			expected: map[string]any{
				"anyOf": []any{
					map[string]any{"enum": []any{"a", "b"}},
					map[string]any{"type": "null"},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := makeNullable(testCase.input); !reflect.DeepEqual(got, testCase.expected) {
				t.Errorf("makeNullable() = %v, want %v", got, testCase.expected)
			}
		})
	}
}
