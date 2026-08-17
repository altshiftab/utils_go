package schema

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cbor"
)

type testPostOrder struct {
	Type                  string `json:"type" jsonschema:"type"`
	LegalBasis            string `json:"legal_basis" jsonschema:"legal_basis"`
	ProjectId             string `json:"project_id,omitzero" jsonschema:"project_id,optional"`
	AdditionalInformation string `json:"additional_information,omitzero" jsonschema:"additional_information,optional"`
}

type testDocument struct {
	FileName string `cborschema:"file_name,minlength:1,maxlength:255"`
	Content  []byte `cborschema:"content,minlength:1"`
}

type testPostPersonOrder struct {
	testPostOrder
	Name         string          `json:"name" jsonschema:"name"`
	EmailAddress string          `json:"email_address" jsonschema:"email_address,format:email"`
	DateOfBirth  string          `json:"date_of_birth,omitzero" jsonschema:"date_of_birth,optional"`
	Documents    []*testDocument `json:"documents,omitzero" jsonschema:"documents,optional,maxitems:10"`
	Priority     int             `cborschema:"priority,optional,minimum:0,maximum:10"`
}

func validOrderValue() map[any]any {
	return map[any]any{
		"type":          "background-check",
		"legal_basis":   "contract",
		"name":          "Meriadoc Brandybuck",
		"email_address": "meriadoc.brandybuck@buckland.example",
		"documents": []any{
			map[any]any{"file_name": "passport.pdf", "content": []byte{0x25, 0x50, 0x44, 0x46}},
		},
		"priority": int64(3),
	}
}

func orderSchema(t *testing.T) *Schema {
	t.Helper()

	derivedSchema, err := NewFromType[*testPostPersonOrder]()
	if err != nil {
		t.Fatalf("new from type: %v", err)
	}

	return derivedSchema
}

func issuesOf(t *testing.T, err error) []*Issue {
	t.Helper()

	if err == nil {
		t.Fatal("expected a validation error")
	}

	var validateError *ValidateError
	if !errors.As(err, &validateError) {
		t.Fatalf("expected a validate error, got %v", err)
	}

	return validateError.Issues
}

func hasIssue(issues []*Issue, path string, messagePart string) bool {
	return slices.ContainsFunc(issues, func(issue *Issue) bool {
		return issue.Path == path && strings.Contains(issue.Message, messagePart)
	})
}

func TestValidateValid(t *testing.T) {
	t.Parallel()

	if err := orderSchema(t).Validate(validOrderValue()); err != nil {
		t.Errorf("validate: %v", err)
	}
}

func TestValidateBytes(t *testing.T) {
	t.Parallel()

	data, err := cbor.Encode(validOrderValue())
	if err != nil {
		t.Fatalf("cbor encode: %v", err)
	}

	if err := orderSchema(t).ValidateBytes(data); err != nil {
		t.Errorf("validate bytes: %v", err)
	}

	if err := orderSchema(t).ValidateBytes([]byte{0x9f}); !errors.Is(err, cbor.ErrMalformed) {
		t.Errorf("expected a malformed error, got %v", err)
	}

	invalidData, err := cbor.Encode(map[any]any{"name": "Meriadoc"})
	if err != nil {
		t.Fatalf("cbor encode: %v", err)
	}

	var validateError *ValidateError
	if err := orderSchema(t).ValidateBytes(invalidData); !errors.As(err, &validateError) {
		t.Errorf("expected a validate error, got %v", err)
	}
}

func TestValidateThroughCodec(t *testing.T) {
	t.Parallel()

	data, err := cbor.Encode(validOrderValue())
	if err != nil {
		t.Fatalf("cbor encode: %v", err)
	}

	value, err := cbor.Decode(data)
	if err != nil {
		t.Fatalf("cbor decode: %v", err)
	}

	if err := orderSchema(t).Validate(value); err != nil {
		t.Errorf("validate: %v", err)
	}
}

func TestValidateViolations(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		mutate      func(value map[any]any)
		path        string
		messagePart string
	}{
		{
			name:        "missing required key",
			mutate:      func(value map[any]any) { delete(value, "name") },
			path:        "",
			messagePart: `missing required key "name"`,
		},
		{
			name:        "unexpected key",
			mutate:      func(value map[any]any) { value["unknown"] = "value" },
			path:        "",
			messagePart: `unexpected key "unknown"`,
		},
		{
			name:        "wrong type",
			mutate:      func(value map[any]any) { value["name"] = int64(5) },
			path:        "/name",
			messagePart: "expected text, got integer",
		},
		{
			name:        "invalid email",
			mutate:      func(value map[any]any) { value["email_address"] = "not-an-email" },
			path:        "/email_address",
			messagePart: "invalid email",
		},
		{
			name: "empty document content",
			mutate: func(value map[any]any) {
				value["documents"] = []any{map[any]any{"file_name": "a.pdf", "content": []byte{}}}
			},
			path:        "/documents/0/content",
			messagePart: "at least 1",
		},
		{
			name: "text instead of bytes",
			mutate: func(value map[any]any) {
				value["documents"] = []any{map[any]any{"file_name": "a.pdf", "content": "JVBERg=="}}
			},
			path:        "/documents/0/content",
			messagePart: "expected bytes, got text",
		},
		{
			name:        "integer out of bounds",
			mutate:      func(value map[any]any) { value["priority"] = int64(11) },
			path:        "/priority",
			messagePart: "at most 10",
		},
		{
			name:        "null for non-nullable",
			mutate:      func(value map[any]any) { value["name"] = nil },
			path:        "/name",
			messagePart: "expected text, got null",
		},
		{
			name:        "non-text map key",
			mutate:      func(value map[any]any) { value[int64(1)] = "value" },
			path:        "",
			messagePart: "non-text key",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			value := validOrderValue()
			testCase.mutate(value)

			issues := issuesOf(t, orderSchema(t).Validate(value))
			if !hasIssue(issues, testCase.path, testCase.messagePart) {
				t.Errorf(
					"expected an issue at %q containing %q, got %v",
					testCase.path,
					testCase.messagePart,
					issues,
				)
			}
		})
	}
}

func TestValidateNullForNullableElement(t *testing.T) {
	t.Parallel()

	value := validOrderValue()
	value["documents"] = []any{nil}

	if err := orderSchema(t).Validate(value); err != nil {
		t.Errorf("validate: %v", err)
	}
}

func TestValidateCollectsMultipleIssues(t *testing.T) {
	t.Parallel()

	value := validOrderValue()
	delete(value, "name")
	value["email_address"] = "not-an-email"
	value["priority"] = int64(-1)

	if issues := issuesOf(t, orderSchema(t).Validate(value)); len(issues) != 3 {
		t.Errorf("expected 3 issues, got %d: %v", len(issues), issues)
	}
}

func TestValidateOptionalOmitted(t *testing.T) {
	t.Parallel()

	value := validOrderValue()
	delete(value, "documents")
	delete(value, "priority")

	if err := orderSchema(t).Validate(value); err != nil {
		t.Errorf("validate: %v", err)
	}
}

func TestValidateNilSchema(t *testing.T) {
	t.Parallel()

	var nilSchema *Schema
	if err := nilSchema.Validate(nil); !errors.Is(err, ErrNilSchema) {
		t.Errorf("expected a nil schema error, got %v", err)
	}
	if err := nilSchema.ValidateBytes(nil); !errors.Is(err, ErrNilSchema) {
		t.Errorf("expected a nil schema error, got %v", err)
	}
}

func TestTypedValidateHelpers(t *testing.T) {
	t.Parallel()

	minLength := 1

	if err := (&Schema{Type: TypeText}).ValidateText("text"); err != nil {
		t.Errorf("validate text: %v", err)
	}

	if err := (&Schema{Type: TypeBytes, MinLength: &minLength}).ValidateByteString([]byte{1}); err != nil {
		t.Errorf("validate byte string: %v", err)
	}
	if err := (&Schema{Type: TypeBytes, MinLength: &minLength}).ValidateByteString([]byte{}); err == nil {
		t.Error("expected a validation error for an empty byte string")
	}

	if err := (&Schema{Type: TypeInteger}).ValidateInteger(3); err != nil {
		t.Errorf("validate integer: %v", err)
	}

	if err := (&Schema{Type: TypeBoolean}).ValidateBoolean(true); err != nil {
		t.Errorf("validate boolean: %v", err)
	}

	if err := (&Schema{Type: TypeArray, Items: &Schema{Type: TypeText}}).ValidateArray([]any{"a"}); err != nil {
		t.Errorf("validate array: %v", err)
	}

	mapSchema := &Schema{
		Type:       TypeMap,
		Properties: map[string]*Schema{"name": {Type: TypeText}},
		Required:   []string{"name"},
	}
	if err := mapSchema.ValidateMap(map[any]any{"name": "Meriadoc"}); err != nil {
		t.Errorf("validate map: %v", err)
	}
	if err := mapSchema.ValidateMap(map[any]any{}); err == nil {
		t.Error("expected a validation error for a missing required key")
	}

	if err := (&Schema{Type: TypeNull}).ValidateNull(); err != nil {
		t.Errorf("validate null: %v", err)
	}
	if err := (&Schema{Type: TypeText}).ValidateNull(); err == nil {
		t.Error("expected a validation error for null against a text schema")
	}
}

func TestNewFromTypeUnsupported(t *testing.T) {
	t.Parallel()

	if _, err := NewFromType[struct {
		Value float64 `json:"value"`
	}](); !errors.Is(err, ErrUnsupportedType) {
		t.Errorf("expected unsupported type error, got %v", err)
	}
}

func TestNewFromTypeRejectsMalformedTag(t *testing.T) {
	t.Parallel()

	if _, err := NewFromType[struct {
		Value string `cborschema:"value,unknown"`
	}](); !errors.Is(err, ErrMalformedTag) {
		t.Errorf("expected malformed tag error, got %v", err)
	}

	if _, err := NewFromType[struct {
		Value string `cborschema:"value,minimum:1"`
	}](); !errors.Is(err, ErrMalformedTag) {
		t.Errorf("expected malformed tag error for value bounds on text, got %v", err)
	}
}

func TestNewFromTypeMapValues(t *testing.T) {
	t.Parallel()

	derivedSchema, err := NewFromType[map[string]int]()
	if err != nil {
		t.Fatalf("new from type: %v", err)
	}

	if err := derivedSchema.Validate(map[any]any{"a": int64(1)}); err != nil {
		t.Errorf("validate: %v", err)
	}

	if err := derivedSchema.Validate(map[any]any{"a": "text"}); err == nil {
		t.Error("expected a validation error for a text map value")
	}
}

func TestArrayElementFormat(t *testing.T) {
	t.Parallel()

	derivedSchema, err := NewFromType[struct {
		Members []string `jsonschema:"members,format:email"`
	}]()
	if err != nil {
		t.Fatalf("new from type: %v", err)
	}

	if err := derivedSchema.Validate(map[any]any{"members": []any{"user@example.com"}}); err != nil {
		t.Errorf("validate: %v", err)
	}

	issues := issuesOf(
		t,
		derivedSchema.Validate(map[any]any{"members": []any{"user@example.com", "bad"}}),
	)
	if !hasIssue(issues, "/members/1", "invalid email") {
		t.Errorf("expected an email issue at /members/1, got %v", issues)
	}
}
