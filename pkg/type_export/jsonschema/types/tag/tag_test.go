package tag

import (
	"errors"
	"slices"
	"testing"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	typeExportErrors "github.com/altshiftab/utils_go/pkg/type_export/errors"
)

func TestNewEmpty(t *testing.T) {
	t.Parallel()

	tag, err := New("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag != nil {
		t.Errorf("expected nil tag for empty input, got %+v", tag)
	}

	// Whitespace-only should also yield nil.
	tag, err = New("   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag != nil {
		t.Errorf("expected nil tag for whitespace input, got %+v", tag)
	}
}

func TestNewSkip(t *testing.T) {
	t.Parallel()

	tag, err := New("-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag == nil {
		t.Fatal("expected non-nil tag")
	}
	if !tag.Skip {
		t.Error("expected Skip=true")
	}
	if tag.Name != "" {
		t.Errorf("expected empty Name on skip tag, got %q", tag.Name)
	}
}

func TestNewNameAndOptional(t *testing.T) {
	t.Parallel()

	tag, err := New("fieldName,optional")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag == nil {
		t.Fatal("expected non-nil tag")
	}
	if tag.Name != "fieldName" {
		t.Errorf("Name = %q, want %q", tag.Name, "fieldName")
	}
	if !tag.Optional {
		t.Error("expected Optional=true")
	}
}

func TestNewValidationConstraints(t *testing.T) {
	t.Parallel()

	tag, err := New("f,minlength:5,maxlength:100,minimum:0.5,maximum:9.5,minitems:2,maxitems:8,format:email")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag == nil {
		t.Fatal("expected non-nil tag")
	}

	if tag.Name != "f" {
		t.Errorf("Name = %q, want %q", tag.Name, "f")
	}
	if tag.MinLength == nil || *tag.MinLength != 5 {
		t.Errorf("MinLength = %v, want 5", tag.MinLength)
	}
	if tag.MaxLength == nil || *tag.MaxLength != 100 {
		t.Errorf("MaxLength = %v, want 100", tag.MaxLength)
	}
	if tag.Minimum == nil || *tag.Minimum != 0.5 {
		t.Errorf("Minimum = %v, want 0.5", tag.Minimum)
	}
	if tag.Maximum == nil || *tag.Maximum != 9.5 {
		t.Errorf("Maximum = %v, want 9.5", tag.Maximum)
	}
	if tag.MinItems == nil || *tag.MinItems != 2 {
		t.Errorf("MinItems = %v, want 2", tag.MinItems)
	}
	if tag.MaxItems == nil || *tag.MaxItems != 8 {
		t.Errorf("MaxItems = %v, want 8", tag.MaxItems)
	}
	if tag.Format != "email" {
		t.Errorf("Format = %q, want %q", tag.Format, "email")
	}
}

func TestNewInvalidNumeric(t *testing.T) {
	t.Parallel()

	cases := []string{
		"f,minlength:abc",
		"f,maxlength:abc",
		"f,minimum:abc",
		"f,maximum:abc",
		"f,minitems:abc",
		"f,maxitems:abc",
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			_, err := New(input)
			if err == nil {
				t.Errorf("expected error for %q, got nil", input)
			}
		})
	}
}

func TestNewUnknownOption(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		tagString string
	}{
		{name: "unknown flag", tagString: "f,unknown_flag"},
		{name: "unknown key and value", tagString: "f,custom:value"},
		{name: "misspelled keyword", tagString: "f,enun:2"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tag, err := New(testCase.tagString)
			if !errors.Is(err, typeExportErrors.ErrUnknownTagOption) {
				t.Fatalf("err = %v, expected %v", err, typeExportErrors.ErrUnknownTagOption)
			}
			if tag != nil {
				t.Errorf("tag = %+v, expected nil", tag)
			}
		})
	}
}

func TestNewEnum(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		tagString string
		expected  []string
	}{
		{name: "one value", tagString: "f,enum:a", expected: []string{"a"}},
		{name: "several values", tagString: "f,enum:a,enum:b,enum:c", expected: []string{"a", "b", "c"}},
		{name: "case preserved", tagString: "f,enum:BankID", expected: []string{"BankID"}},
		{name: "keyword case-insensitive", tagString: "f,ENUM:x", expected: []string{"x"}},
		{name: "value with a colon", tagString: "f,enum:a:b", expected: []string{"a:b"}},
		{name: "with other options", tagString: "f,optional,enum:a,format:uuid", expected: []string{"a"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tag, err := New(testCase.tagString)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tag == nil {
				t.Fatal("expected non-nil tag")
			}
			if !slices.Equal(tag.Enum, testCase.expected) {
				t.Errorf("Enum = %v, expected %v", tag.Enum, testCase.expected)
			}
		})
	}
}

func TestNewEnumEmptyValue(t *testing.T) {
	t.Parallel()

	if _, err := New("f,enum:"); err == nil {
		t.Fatal("expected an error for an empty enum value")
	} else if _, ok := errors.AsType[*empty_error.Error](err); !ok {
		t.Fatalf("err = %v, expected an empty error", err)
	}
}

func TestNewAdditionalProperties(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		tagString   string
		expectError bool
		expected    *bool
	}{
		{name: "true on a blank field", tagString: ",additionalProperties:true", expected: ptr(true)},
		{name: "false on a blank field", tagString: ",additionalProperties:false", expected: ptr(false)},
		{name: "named field", tagString: "fieldName,additionalProperties:true", expected: ptr(true)},
		{name: "case insensitive", tagString: ",ADDITIONALPROPERTIES:TRUE", expected: ptr(true)},
		{name: "unset when unsaid", tagString: "fieldName,optional", expected: nil},
		{name: "not a bool", tagString: ",additionalProperties:perhaps", expectError: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tag, err := New(testCase.tagString)
			if testCase.expectError {
				if err == nil {
					t.Fatalf("expected an error for %q", testCase.tagString)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tag == nil {
				t.Fatal("expected non-nil tag")
			}

			if testCase.expected == nil {
				if tag.AdditionalProperties != nil {
					t.Errorf("additional properties: got %t, want unset", *tag.AdditionalProperties)
				}
			} else {
				if tag.AdditionalProperties == nil {
					t.Fatalf("additional properties: got unset, want %t", *testCase.expected)
				}
				if *tag.AdditionalProperties != *testCase.expected {
					t.Errorf("additional properties: got %t, want %t", *tag.AdditionalProperties, *testCase.expected)
				}
			}
		})
	}
}

func ptr[T any](value T) *T {
	return &value
}
