// Package schema implements validation of decoded CBOR values (the value model produced by
// github.com/altshiftab/utils_go/pkg/cbor) against schemas. Schemas can be authored directly or
// derived from Go types with NewFromType, using the same struct tag grammar as the jsonschema
// library. The keyword set is deliberately small; unknown map keys are rejected unless
// AdditionalProperties is set.
package schema

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/altshiftab/utils_go/pkg/cbor"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

var (
	ErrUnsupportedType = errors.New("unsupported type")
	ErrMalformedTag    = errors.New("malformed tag")
	ErrNilSchema       = errors.New("nil schema")
)

type Type string

const (
	// TypeAny imposes no type constraint.
	TypeAny     Type = ""
	TypeMap     Type = "map"
	TypeArray   Type = "array"
	TypeText    Type = "text"
	TypeBytes   Type = "bytes"
	TypeInteger Type = "integer"
	TypeBoolean Type = "boolean"
	TypeNull    Type = "null"
)

type Schema struct {
	Type Type
	// Nullable additionally permits null regardless of Type.
	Nullable bool

	// Properties describes map entries by text key (Type = TypeMap).
	Properties map[string]*Schema
	// Required lists property keys that must be present (Type = TypeMap).
	Required []string
	// AdditionalProperties describes map entries not named in Properties (Type = TypeMap). When
	// nil, such entries are rejected.
	AdditionalProperties *Schema

	// Items describes array elements (Type = TypeArray).
	Items    *Schema
	MinItems *int
	MaxItems *int

	// MinLength and MaxLength bound text length in runes and bytes length in bytes
	// (Type = TypeText or TypeBytes).
	MinLength *int
	MaxLength *int

	// Minimum and Maximum bound integers inclusively (Type = TypeInteger).
	Minimum *int64
	Maximum *int64

	// Format names a registered format validator (Type = TypeText).
	Format string
}

// Issue is a single validation failure at the value identified by Path (a JSON Pointer-style
// path; empty for the root value).
type Issue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (issue *Issue) String() string {
	path := issue.Path
	if path == "" {
		path = "(root)"
	}
	return fmt.Sprintf("%s: %s", path, issue.Message)
}

type ValidateError struct {
	Issues []*Issue
}

func (validateError *ValidateError) Error() string {
	messages := make([]string, len(validateError.Issues))
	for i, issue := range validateError.Issues {
		messages[i] = issue.String()
	}
	return fmt.Sprintf("validation failed: %s", strings.Join(messages, "; "))
}

func typeName(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case int64:
		return "integer"
	case string:
		return "text"
	case []byte:
		return "bytes"
	case []any:
		return "array"
	case map[any]any:
		return "map"
	case cbor.Tag:
		return "tag"
	case cbor.Undefined:
		return "undefined"
	default:
		return fmt.Sprintf("%T", value)
	}
}

func (s *Schema) matchesType(value any) bool {
	switch s.Type {
	case TypeAny:
		return true
	case TypeMap:
		_, ok := value.(map[any]any)
		return ok
	case TypeArray:
		_, ok := value.([]any)
		return ok
	case TypeText:
		_, ok := value.(string)
		return ok
	case TypeBytes:
		_, ok := value.([]byte)
		return ok
	case TypeInteger:
		_, ok := value.(int64)
		return ok
	case TypeBoolean:
		_, ok := value.(bool)
		return ok
	case TypeNull:
		return value == nil
	default:
		return false
	}
}

func (s *Schema) validateValue(value any, path string, issues *[]*Issue) {
	if s == nil {
		return
	}

	addIssue := func(format string, arguments ...any) {
		*issues = append(*issues, &Issue{Path: path, Message: fmt.Sprintf(format, arguments...)})
	}

	if value == nil {
		if s.Nullable || s.Type == TypeNull || s.Type == TypeAny {
			return
		}
		addIssue("expected %s, got null", s.Type)
		return
	}

	if !s.matchesType(value) {
		addIssue("expected %s, got %s", s.Type, typeName(value))
		return
	}

	switch typedValue := value.(type) {
	case map[any]any:
		for _, requiredKey := range s.Required {
			if _, ok := typedValue[requiredKey]; !ok {
				addIssue("missing required key %q", requiredKey)
			}
		}

		for key, item := range typedValue {
			textKey, ok := key.(string)
			if !ok {
				addIssue("unexpected non-text key %v", key)
				continue
			}

			itemPath := path + "/" + textKey

			if propertySchema, ok := s.Properties[textKey]; ok {
				propertySchema.validateValue(item, itemPath, issues)
				continue
			}

			if additionalPropertiesSchema := s.AdditionalProperties; additionalPropertiesSchema != nil {
				additionalPropertiesSchema.validateValue(item, itemPath, issues)
				continue
			}

			addIssue("unexpected key %q", textKey)
		}
	case []any:
		if minItems := s.MinItems; minItems != nil && len(typedValue) < *minItems {
			addIssue("expected at least %d items, got %d", *minItems, len(typedValue))
		}
		if maxItems := s.MaxItems; maxItems != nil && len(typedValue) > *maxItems {
			addIssue("expected at most %d items, got %d", *maxItems, len(typedValue))
		}

		if itemsSchema := s.Items; itemsSchema != nil {
			for i, item := range typedValue {
				itemsSchema.validateValue(item, path+"/"+strconv.Itoa(i), issues)
			}
		}
	case string:
		length := utf8.RuneCountInString(typedValue)
		if minLength := s.MinLength; minLength != nil && length < *minLength {
			addIssue("expected a length of at least %d, got %d", *minLength, length)
		}
		if maxLength := s.MaxLength; maxLength != nil && length > *maxLength {
			addIssue("expected a length of at most %d, got %d", *maxLength, length)
		}

		if format := s.Format; format != "" {
			formatValidator, ok := formatRegistry[format]
			if !ok {
				addIssue("unknown format %q", format)
			} else if err := formatValidator(typedValue); err != nil {
				addIssue("invalid %s: %v", format, err)
			}
		}
	case []byte:
		if minLength := s.MinLength; minLength != nil && len(typedValue) < *minLength {
			addIssue("expected a length of at least %d, got %d", *minLength, len(typedValue))
		}
		if maxLength := s.MaxLength; maxLength != nil && len(typedValue) > *maxLength {
			addIssue("expected a length of at most %d, got %d", *maxLength, len(typedValue))
		}
	case int64:
		if minimum := s.Minimum; minimum != nil && typedValue < *minimum {
			addIssue("expected a value of at least %d, got %d", *minimum, typedValue)
		}
		if maximum := s.Maximum; maximum != nil && typedValue > *maximum {
			addIssue("expected a value of at most %d, got %d", *maximum, typedValue)
		}
	}
}

// Validate checks a decoded CBOR value against the schema, returning a *ValidateError carrying
// all violations, or nil if the value is valid. The value must use the type model produced by
// cbor.Decode: map[any]any, []any, string, []byte, int64, bool, nil, cbor.Tag, or cbor.Undefined.
func (s *Schema) Validate(value any) error {
	if s == nil {
		return altshiftErrors.NewWithTrace(ErrNilSchema)
	}

	var issues []*Issue
	s.validateValue(value, "", &issues)

	if len(issues) != 0 {
		return &ValidateError{Issues: issues}
	}

	return nil
}

// ValidateBytes decodes data and validates the resulting value. When the decoded value is also
// needed afterwards (for cbor.UnmarshalValue, say), decode once and use Validate instead of
// decoding twice.
func (s *Schema) ValidateBytes(data []byte) error {
	if s == nil {
		return altshiftErrors.NewWithTrace(ErrNilSchema)
	}

	// Nothing decoded escapes into the returned issues, so the byte strings can alias data.
	value, err := cbor.DecodeNoCopy(data)
	if err != nil {
		return fmt.Errorf("cbor decode: %w", err)
	}

	return s.Validate(value)
}

// ValidateMap validates a map value.
func (s *Schema) ValidateMap(value map[any]any) error {
	return s.Validate(value)
}

// ValidateArray validates an array value.
func (s *Schema) ValidateArray(value []any) error {
	return s.Validate(value)
}

// ValidateText validates a text-string value.
func (s *Schema) ValidateText(value string) error {
	return s.Validate(value)
}

// ValidateByteString validates a byte-string value. Unlike ValidateBytes, the value is not
// decoded; it is the value.
func (s *Schema) ValidateByteString(value []byte) error {
	return s.Validate(value)
}

// ValidateInteger validates an integer value.
func (s *Schema) ValidateInteger(value int64) error {
	return s.Validate(value)
}

// ValidateBoolean validates a boolean value.
func (s *Schema) ValidateBoolean(value bool) error {
	return s.Validate(value)
}

// ValidateNull validates the null value.
func (s *Schema) ValidateNull() error {
	return s.Validate(nil)
}

// Clone returns a deep copy of the schema.
func (s *Schema) Clone() *Schema {
	if s == nil {
		return nil
	}

	clone := *s

	if s.Properties != nil {
		clone.Properties = make(map[string]*Schema, len(s.Properties))
		for key, propertySchema := range s.Properties {
			clone.Properties[key] = propertySchema.Clone()
		}
	}
	clone.Required = slices.Clone(s.Required)
	clone.AdditionalProperties = s.AdditionalProperties.Clone()
	clone.Items = s.Items.Clone()

	return &clone
}
