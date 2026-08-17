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
