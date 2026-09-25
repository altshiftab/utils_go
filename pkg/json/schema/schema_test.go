package schema

import (
	"errors"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

func TestNew(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name       string
		schemaJSON string
		wantErr    bool
	}{
		{
			name:       "valid schema",
			schemaJSON: `{"type": "object", "properties": {"name": {"type": "string"}}}`,
		},
		{
			name:       "invalid JSON",
			schemaJSON: `{`,
			wantErr:    true,
		},
		{
			name:       "invalid keyword argument",
			schemaJSON: `{"minLength": "x"}`,
			wantErr:    true,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s, err := New([]byte(testCase.schemaJSON))
			if (err != nil) != testCase.wantErr {
				t.Fatalf("New: error %v, wantErr %t", err, testCase.wantErr)
			}
			if err == nil && s == nil {
				t.Error("New returned nil schema and nil error")
			}
		})
	}
}

func TestNewValidate(t *testing.T) {
	t.Parallel()
	s, err := New([]byte(`{
		"type": "object",
		"properties": {"name": {"type": "string", "minLength": 1}},
		"required": ["name"]
	}`))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "valid", instance: map[string]any{"name": "x"}},
		{name: "missing required", instance: map[string]any{}, wantErr: true},
		{name: "wrong type", instance: map[string]any{"name": 5}, wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := s.Validate(testCase.instance)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("Validate: error %v, wantErr %t", err, testCase.wantErr)
			}
			if err != nil && !errors.Is(err, altshiftErrors.ErrValidationError) {
				t.Error("errors.Is(err, altshiftErrors.ErrValidationError) = false, want true")
			}
		})
	}
}

func TestNewFromType(t *testing.T) {
	t.Parallel()
	type person struct {
		Name string `json:"name"`
		Age  int    `json:"age,omitzero"`
	}

	s, err := NewFromType[person]()
	if err != nil {
		t.Fatalf("NewFromType: %v", err)
	}

	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "valid", instance: map[string]any{"name": "x", "age": float64(30)}},
		{name: "optional omitted", instance: map[string]any{"name": "x"}},
		{name: "wrong type", instance: map[string]any{"name": float64(5)}, wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := s.Validate(testCase.instance)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("Validate(%v): error %v, wantErr %t", testCase.instance, err, testCase.wantErr)
			}
		})
	}
}

type schemaTestRole string

func (schemaTestRole) EnumValues() []string { return []string{"admin", "dd"} }

func TestNewFromTypeEnum(t *testing.T) {
	t.Parallel()
	type account struct {
		Roles []schemaTestRole `json:"roles"`
		Kind  *string          `json:"kind" jsonschema:"kind,enum:bankid,enum:passport"`
	}

	s, err := NewFromType[account]()
	if err != nil {
		t.Fatalf("NewFromType: %v", err)
	}

	testCases := []struct {
		name     string
		instance any
		wantErr  bool
	}{
		{name: "valid", instance: map[string]any{"roles": []any{"admin", "dd"}, "kind": "bankid"}},
		{name: "null kind", instance: map[string]any{"roles": []any{"dd"}, "kind": nil}},
		{name: "unknown role", instance: map[string]any{"roles": []any{"this_is_not_a_valid_role"}, "kind": "bankid"}, wantErr: true},
		{name: "unknown kind", instance: map[string]any{"roles": []any{"dd"}, "kind": "not_a_type"}, wantErr: true},
		{name: "kind in another case", instance: map[string]any{"roles": []any{"dd"}, "kind": "BankID"}, wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := s.Validate(testCase.instance)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("Validate(%v): error %v, wantErr %t", testCase.instance, err, testCase.wantErr)
			}
			if err != nil {
				if _, ok := errors.AsType[*ValidateError](err); !ok {
					t.Errorf("error is not a ValidateError: %v", err)
				}
			}
		})
	}
}

func TestValidateKeywordLocation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                     string
		schema                   string
		instance                 any
		expectedKeywordLocation  string
		expectedInstanceLocation string
	}{
		{
			name:                     "items under properties",
			schema:                   `{"type": "object", "properties": {"roles": {"type": "array", "items": {"enum": ["a"]}}}}`,
			instance:                 map[string]any{"roles": []any{"a", "b"}},
			expectedKeywordLocation:  "#/properties/roles/items/enum",
			expectedInstanceLocation: "#/roles/1",
		},
		{
			name:                     "prefixItems",
			schema:                   `{"type": "array", "prefixItems": [{"type": "string"}, {"type": "integer"}]}`,
			instance:                 []any{"a", "b"},
			expectedKeywordLocation:  "#/prefixItems/1/type",
			expectedInstanceLocation: "#/1",
		},
		{
			name:                     "items after prefixItems",
			schema:                   `{"type": "array", "prefixItems": [{"type": "string"}], "items": {"type": "integer"}}`,
			instance:                 []any{"a", "b"},
			expectedKeywordLocation:  "#/items/type",
			expectedInstanceLocation: "#/1",
		},
		{
			name:                     "then",
			schema:                   `{"if": {"type": "string"}, "then": {"minLength": 2}, "else": {"type": "integer"}}`,
			instance:                 "a",
			expectedKeywordLocation:  "#/then/minLength",
			expectedInstanceLocation: "#",
		},
		{
			name:                     "else",
			schema:                   `{"if": {"type": "string"}, "then": {"minLength": 2}, "else": {"type": "integer"}}`,
			instance:                 true,
			expectedKeywordLocation:  "#/else/type",
			expectedInstanceLocation: "#",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			s, err := New([]byte(testCase.schema))
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			validateError, ok := errors.AsType[*ValidateError](s.Validate(testCase.instance))
			if !ok || len(validateError.Errors) != 1 {
				t.Fatalf("expected one validation error, got %v", validateError)
			}

			got := validateError.Errors[0]
			if got.KeywordLocation != testCase.expectedKeywordLocation {
				t.Errorf("keyword location = %q, expected %q", got.KeywordLocation, testCase.expectedKeywordLocation)
			}
			if got.InstanceLocation != testCase.expectedInstanceLocation {
				t.Errorf("instance location = %q, expected %q", got.InstanceLocation, testCase.expectedInstanceLocation)
			}
		})
	}
}
