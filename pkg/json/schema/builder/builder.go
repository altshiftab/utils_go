// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package builder defines a [Builder] type that may be used
// to build a [jsonschema.Schema] step by step.
//
// It is usually more convenient to use the Builder defined by
// the specific JSON schema draft that you are using.
//
// The [Infer] and [InferType] functions may be used with a Builder
// to build a schema from a Go type.
package builder

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/altshiftab/utils_go/pkg/json/schema/internal/argtype"
	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

// Builder is a JSON schema builder.
// Builder provides a list of methods that may be used to add
// new elements to the schema.
// This should be used by programs that need to create a JSON schema
// from scratch, rather than unmarshaling it from a JSON representation.
//
// When using Builder there is no support for references to other schemas
// via $ref or $dynamicRef. Similarly there is no way to define anchors
// via $anchor, $dynamicAnchor, or $defs.
type Builder struct {
	s schema.Schema
	v *schema.Vocabulary
}

// New returns a new [Builder] to build a [*types.Schema]
// described by the [*types.Vocabulary] v.
func New(v *schema.Vocabulary) *Builder {
	return &Builder{v: v}
}

// Build builds and returns the [*jsonschema.Schema].
func (b *Builder) Build() *schema.Schema {
	s := b.s
	s.Finalize(b.v)
	return &s
}

// NewBuilder returns a new Builder with the same vocabulary.
func (b *Builder) NewBuilder() *Builder {
	return New(b.v)
}

// AddBool adds a keyword whose argument is a bool.
// This panics if the keyword does not expect a bool.
func (b *Builder) AddBool(keyword *schema.Keyword, v bool) *Builder {
	b.check(keyword, schema.ArgTypeBool)
	b.s.Parts = append(b.s.Parts, schema.MakePart(keyword, schema.PartBool(v)))
	return b
}

// AddString adds a keyword whose argument is a string.
func (b *Builder) AddString(keyword *schema.Keyword, s string) *Builder {
	if keyword.ArgType == schema.ArgTypeStringOrStrings {
		b.s.Parts = append(b.s.Parts, schema.MakePart(keyword, schema.PartStringOrStrings{String: s}))
	} else {
		b.check(keyword, schema.ArgTypeString)
		b.s.Parts = append(b.s.Parts, schema.MakePart(keyword, schema.PartString(s)))
	}
	return b
}

// AddStrings adds a keyword whose argument is an array of strings.
func (b *Builder) AddStrings(keyword *schema.Keyword, s []string) *Builder {
	if keyword.ArgType == schema.ArgTypeStringOrStrings {
		b.s.Parts = append(b.s.Parts, schema.MakePart(keyword, schema.PartStringOrStrings{Strings: s}))
	} else {
		b.check(keyword, schema.ArgTypeStrings)
		b.s.Parts = append(b.s.Parts, schema.MakePart(keyword, schema.PartStrings(s)))
	}
	return b
}

// AddInt adds a keyword whose argument is an int.
func (b *Builder) AddInt(keyword *schema.Keyword, i int64) *Builder {
	b.check(keyword, schema.ArgTypeInt)
	b.s.Parts = append(b.s.Parts, schema.MakePart(keyword, schema.PartInt(i)))
	return b
}

// AddFloat adds a keyword whose argument is an float.
func (b *Builder) AddFloat(keyword *schema.Keyword, f float64) *Builder {
	b.check(keyword, schema.ArgTypeFloat)
	b.s.Parts = append(b.s.Parts, schema.MakePart(keyword, schema.PartFloat(f)))
	return b
}

// AddSchema adds a keyword whose argument is a schema.
// This panics if the schema is nil.
func (b *Builder) AddSchema(keyword *schema.Keyword, s *schema.Schema) *Builder {
	b.check(keyword, schema.ArgTypeSchema)
	if s == nil {
		panic(fmt.Sprintf("%s schema is nil", keyword.Name))
	}
	b.s.Parts = append(b.s.Parts, schema.MakePart(keyword, schema.PartSchema{S: s}))
	return b
}

// AddSchemas adds a keyword whose argument is a list of schemas.
// This panics if the list of schemas is empty or any is nil.
// This may be used to implement a custom schema keyword.
func (b *Builder) AddSchemas(keyword *schema.Keyword, schemas []*schema.Schema) *Builder {
	b.check(keyword, schema.ArgTypeSchemas)
	if len(schemas) == 0 {
		panic(fmt.Sprintf("%s requires at least one schema", keyword.Name))
	}
	for i, s := range schemas {
		if s == nil {
			panic(fmt.Sprintf("%s schema %d is nil", keyword.Name, i))
		}
	}
	b.s.Parts = append(b.s.Parts, schema.MakePart(keyword, schema.PartSchemas(schemas)))
	return b
}

// AddMapSchema adds a keyword whose argument is a mapping
// from strings to schemas.
func (b *Builder) AddMapSchema(keyword *schema.Keyword, m map[string]*schema.Schema) *Builder {
	b.check(keyword, schema.ArgTypeMapSchema)
	b.s.Parts = append(b.s.Parts, schema.MakePart(keyword, schema.PartMapSchema(m)))
	return b
}

// AddSchemaOrSchemas adds a keyword whose argument is
// either a single schema or an array of schemas.
func (b *Builder) AddSchemaOrSchemas(keyword *schema.Keyword, pv schema.PartSchemaOrSchemas) *Builder {
	b.check(keyword, schema.ArgTypeSchemaOrSchemas)
	b.s.Parts = append(b.s.Parts, schema.MakePart(keyword, pv))
	return b
}

// AddMapArrayOrSchema adds a keyword whose argument is
// a map from strings to either arrays or schemas.
// This is like the draft7 "dependencies" keyword.
// This probably should not be used for anything else.
func (b *Builder) AddMapArrayOrSchema(keyword *schema.Keyword, pv schema.PartMapArrayOrSchema) *Builder {
	b.check(keyword, schema.ArgTypeMapArrayOrSchema)
	b.s.Parts = append(b.s.Parts, schema.MakePart(keyword, pv))
	return b
}

// AddAny adds a keyword whose argument has any type.
func (b *Builder) AddAny(keyword *schema.Keyword, v any) *Builder {
	b.s.Parts = append(b.s.Parts, schema.MakePart(keyword, schema.PartAny{V: v}))
	return b
}

// typeNameInteger and typeNameObject are the JSON type names that
// schema inference uses more than once.
const (
	typeNameInteger = "integer"
	typeNameObject  = "object"
)

// check panics if a keyword is used with the wrong type.
func (b *Builder) check(keyword *schema.Keyword, want schema.ArgType) {
	//nolint:exhaustive // Any argument type other than the wanted one is a programming error.
	switch keyword.ArgType {
	case want, schema.ArgTypeAny:
	default:
		argName := argtype.Name(keyword.ArgType)
		panic(fmt.Sprintf("Add%s called for %s which expects %s", argName, keyword.Name, argName))
	}
}

// AddSchemaParts adds a list of parts.
func (b *Builder) AddSchemaParts(parts []schema.Part) *Builder {
	b.s.Parts = append(b.s.Parts, parts...)
	return b
}

// InferOpts contains options to pass when inferring a JSON schema.
type InferOpts struct {
	// Types maps types to the schema to infer for values of those types.
	// The key is a type,
	// the value is the schema to use for values of that type.
	// This overrides any default inferences;
	// mapping to nil uses the default behavior for that type.
	Types map[reflect.Type]*schema.Schema

	// If IgnoreInvalidTypes is true, fields that can't be represented
	// in a JSON schema are ignored. For example, fields of
	// function type. The caller can add describe these fields using
	// the returned Builder.
	IgnoreInvalidTypes bool
}

// Infer adds schema elements to b designed to validate JSON values
// that unmarshal into values of the given type.
//
// The default translation is:
//
//   - Strings become "type":"string".
//   - Bools become "type":"bool".
//   - Integer types become "type":"integer".
//   - Floating point types become "type":"number".
//   - Slice and array types become "type":"array",
//     with an "items" entry mapped to a schema inferred
//     from the element type.
//     An array type will have "minItems" and "maxItems" set to the
//     length of the array.
//   - Maps with a string key become "type":"object",
//     with an "additionalProperties" entry mapped to a schema inferred
//     from the value type.
//   - Structs have "type":"object", and include "properties"
//     for each exported field using the JSON name of the field.
//     Fields ignored by the JSON marshaler are ignored here.
//     Fields whose JSON attributes include neither "omitempty" nor "omitzero"
//     are added to a "required" list.
//   - Interface types are accepted but add nothing to the schema.
//   - Some standard library types with custom JSOM marshaling
//     are translated to predefined schemas.
//     This may be overridden using the [InferOpts.Types] option.
//
// For other Go types Infer will return an error.
// Other types may be handled specially using the [InferOpts.Types] option.
//
// Infer will look at jsonschema struct field tags.
// The tag may start with keyword=value pairs separated by commas,
// where a keyword does not contain space or tab characters.
// If the tag, or the trailing part of the tag, does not contain =,
// that will set the "description" property.
// Recognized tag keywords are:
//
//	enum=A,enum=B,... sets the "enum" property to the listed values
//
// As this function takes and returns a [Builder], the caller may
// add additional schema checks before calling the Build method
// to get a schema.
func Infer[T any, Builder inferBuilder[Builder]](builder Builder, opts *InferOpts) (Builder, error) {
	return InferType(builder, reflect.TypeFor[T](), opts)
}

// InferType is like [Infer] but takes a [reflect.Type] rather than
// a type argument.
func InferType[Builder inferBuilder[Builder]](builder Builder, typ reflect.Type, opts *InferOpts) (Builder, error) {
	return inferType[Builder](builder, typ, make(map[reflect.Type]bool), opts)
}

// inferBuilder is an interface used as a constraint by [Infer].
// This lets Infer accept a version-specific Builder type.
type inferBuilder[Builder any] interface {
	AddSchemaParts([]schema.Part) Builder
	AddType(args ...string) Builder
	AddItemsSchema(*schema.Schema) Builder
	AddMinItems(int64) Builder
	AddMaxItems(int64) Builder
	AddMinimum(float64) Builder
	AddMaximum(float64) Builder
	AddProperties(map[string]*schema.Schema) Builder
	AddAdditionalProperties(*schema.Schema) Builder
	AddRequired([]string) Builder
	AddEnum(any) Builder
	AddDescription(string) Builder
	NewSubBuilder() Builder
	BoolSchema(bool) *schema.Schema
	Build() *schema.Schema
}

// inferType implements Infer, using a map to detect type cycles.
func inferType[Builder inferBuilder[Builder]](builder Builder, typ reflect.Type, seen map[reflect.Type]bool, opts *InferOpts) (Builder, error) {
	var z Builder

	isPointer := false
	for typ.Kind() == reflect.Pointer {
		if opts != nil {
			s, schemaSet := opts.Types[typ]
			if schemaSet {
				return addParts(builder, s, isPointer), nil
			}
		}

		isPointer = true
		typ = typ.Elem()
	}

	if typ.Name() != "" {
		if seen[typ] {
			return z, fmt.Errorf("%w: type cycle at %s", errors.ErrUnsupported, typ)
		}
		seen[typ] = true
		defer delete(seen, typ)
	}

	if opts != nil {
		s, schemaSet := opts.Types[typ]
		if schemaSet {
			return addParts(builder, s, isPointer), nil
		}
	}

	switch typ {
	case reflect.TypeFor[time.Time](), reflect.TypeFor[slog.Level](), reflect.TypeFor[big.Rat](), reflect.TypeFor[big.Float]():
		return builder.AddType("string"), nil
	case reflect.TypeFor[big.Int]():
		return builder.AddType("null", "string"), nil
	}

	addType := ""
	//nolint:exhaustive // The default case rejects the remaining kinds.
	switch typ.Kind() {
	case reflect.Bool:
		addType = "boolean"

	case reflect.Int, reflect.Int64:
		addType = typeNameInteger

	case reflect.Uint, reflect.Uint64, reflect.Uintptr:
		addType = typeNameInteger
		builder.AddMinimum(0)

	case reflect.Int8:
		addType = typeNameInteger
		builder.AddMinimum(math.MinInt8)
		builder.AddMaximum(math.MaxInt8)

	case reflect.Uint8:
		addType = typeNameInteger
		builder.AddMinimum(0)
		builder.AddMaximum(math.MaxUint8)

	case reflect.Int16:
		addType = typeNameInteger
		builder.AddMinimum(math.MinInt16)
		builder.AddMaximum(math.MaxInt16)

	case reflect.Uint16:
		addType = typeNameInteger
		builder.AddMinimum(0)
		builder.AddMaximum(math.MaxUint16)

	case reflect.Int32:
		addType = typeNameInteger
		builder.AddMinimum(math.MinInt32)
		builder.AddMaximum(math.MaxInt32)

	case reflect.Uint32:
		addType = typeNameInteger
		builder.AddMinimum(0)
		builder.AddMaximum(math.MaxUint32)

	case reflect.Float32, reflect.Float64:
		addType = "number"

	case reflect.String:
		addType = "string"

	case reflect.Interface:
		// Nothing to do.

	case reflect.Map:
		addType = typeNameObject
		if typ.Key().Kind() != reflect.String {
			if opts != nil && opts.IgnoreInvalidTypes {
				return z, nil
			}
			return z, fmt.Errorf("%w: map key type %s", errors.ErrUnsupported, typ.Key())
		}
		be := builder.NewSubBuilder()
		be, err := inferType[Builder](be, typ.Elem(), seen, opts)
		if err != nil {
			return z, fmt.Errorf("map value schema: %w", err)
		}
		if !reflect.ValueOf(be).IsZero() {
			builder = builder.AddAdditionalProperties(be.Build())
		}

	case reflect.Slice, reflect.Array:
		addType = "array"
		be := builder.NewSubBuilder()
		be, err := inferType[Builder](be, typ.Elem(), seen, opts)
		if err != nil {
			return z, fmt.Errorf("slice/array element schema: %w", err)
		}
		if !reflect.ValueOf(be).IsZero() {
			builder = builder.AddItemsSchema(be.Build())
			if typ.Kind() == reflect.Array {
				ln := int64(typ.Len())
				builder = builder.AddMinItems(ln)
				builder = builder.AddMaxItems(ln)
			}
		}

	case reflect.Struct:
		addType = typeNameObject

		var properties map[string]*schema.Schema
		var required []string
		fields := reflect.VisibleFields(typ)
		for i := 0; i < len(fields); i++ {
			field := fields[i]

			// We can ignore anonymous fields,
			// unless they have an entry in opts.Types.
			if field.Anonymous {
				if opts == nil {
					continue
				}
				s, schemaSet := opts.Types[field.Type]
				if !schemaSet {
					continue
				}

				// We only permit type object,
				// with properties.
				sawType := false
				for _, part := range s.Parts {
					if part.Keyword.Generated {
						continue
					}
					switch part.Keyword.Name {
					case "$schema":
						// ignore
					case "type":
						if part.Value.(schema.PartStringOrStrings).String != typeNameObject {
							return z, fmt.Errorf(`%w: custom schema for embedded field must have type "object", got %q`, errors.ErrUnsupported, part.Value)
						}
						sawType = true
					case "properties":
						if properties == nil {
							properties = make(map[string]*schema.Schema)
						}
						for n, s := range part.Value.(schema.PartMapSchema) {
							properties[n] = s.Clone()
						}
					default:
						return z, fmt.Errorf(`%w: override for embedded field can only have "type" and "properties"; this has %q`, errors.ErrUnsupported, part.Keyword.Name)
					}
				}
				if !sawType {
					return z, fmt.Errorf(`%w: custom schema for embedded field must have type "object", no type given`, errors.ErrUnsupported)
				}

				// Since we have a schema, skip the fields.
				index := field.Index
				indLen := len(index)
				for i++; i < len(fields); i++ {
					if len(fields[i].Index) <= indLen {
						break
					}
					if !slices.Equal(fields[i].Index[:indLen], index) {
						break
					}
				}
				i-- // undone by fields loop increment

				continue
			}

			name, omit, optional := fieldJSON(&field)
			if omit {
				continue
			}

			bf := builder.NewSubBuilder()
			bf, err := inferType[Builder](bf, field.Type, seen, opts)
			if err != nil {
				return z, fmt.Errorf("field %s.%s schema: %w", typ, field.Name, err)
			}
			if reflect.ValueOf(bf).IsZero() {
				continue
			}

			if tag, ok := field.Tag.Lookup("jsonschema"); ok {
				bf, err = addFieldTag(bf, tag)
				if err != nil {
					return z, fmt.Errorf("field %s.%s: %w", typ, field.Name, err)
				}
			}

			bs := bf.Build()

			if properties == nil {
				properties = make(map[string]*schema.Schema)
			}
			properties[name] = bs

			if !optional {
				required = append(required, name)
			}
		}

		if properties != nil {
			builder = builder.AddProperties(properties)
		}

		if len(required) > 0 {
			builder = builder.AddRequired(required)
		}

		// No unknown fields may be specified.
		falseSchema := builder.BoolSchema(false)
		builder = builder.AddAdditionalProperties(falseSchema)

	default:
		if opts != nil && opts.IgnoreInvalidTypes {
			return z, nil
		}

		return z, fmt.Errorf("%w: jsonschema type %s", errors.ErrUnsupported, typ)
	}

	if addType != "" {
		if isPointer {
			builder = builder.AddType("null", addType)
		} else {
			builder = builder.AddType(addType)
		}
	}

	return builder, nil
}

// fieldJSON reports the JSON name of a struct field, whether the
// field is omitted, and whether it is optional.
func fieldJSON(sf *reflect.StructField) (string, bool, bool) {
	if !sf.IsExported() {
		// Omit unexported field.
		return "", true, false
	}

	tag, ok := sf.Tag.Lookup("json")
	if !ok {
		// No tag means use the field name as the JSON name.
		return sf.Name, false, false
	}

	if tag == "-" {
		// Omit field.
		return "", true, false
	}

	// Fetch the JSON name from the tag.
	name, opts, _ := strings.Cut(tag, ",")
	if name == "" {
		name = sf.Name
	}

	// The field is optional if it has a omitzero or omitempty tag.
	optional := false
	for opts != "" {
		var opt string
		opt, opts, _ = strings.Cut(opts, ",")
		if opt == "omitzero" || opt == "omitempty" {
			optional = true
			break
		}
	}

	return name, false, optional
}

// addParts adds the parts of s to builder.
// If addNull is true then any existing "type" attribute
// is modified to also permit "null".
func addParts[Builder inferBuilder[Builder]](builder Builder, s *schema.Schema, addNull bool) Builder {
	parts := s.Parts
	if addNull {
		for i, part := range parts {
			if part.Keyword.Name != "type" {
				continue
			}
			pv := part.Value.(schema.PartStringOrStrings)
			if pv.String == "null" || slices.Contains(pv.Strings, "null") {
				break
			}
			parts := slices.Clone(parts)
			if pv.String == "" {
				pv.Strings = append(pv.Strings, "null")
			} else {
				pv.Strings = []string{"null", pv.String}
				pv.String = ""
			}
			//nolint:nilaway // slices.Clone returns nil only for a nil slice, in which case this loop does not run.
			parts[i].Value = pv
			break
		}
	}
	builder.AddSchemaParts(parts)
	return builder
}

// addFieldTag parses the jsonschema field tag and adds elements to builder.
func addFieldTag[Builder inferBuilder[Builder]](builder Builder, tag string) (Builder, error) {
	if tag == "" {
		return builder, fmt.Errorf("%w: empty jsonschema tag", errors.ErrUnsupported)
	}

	var enums []any
	for tag != "" {
		keyword, tail, ok := strings.Cut(tag, "=")

		if !ok || strings.ContainsAny(keyword, " \t") {
			builder.AddDescription(tag)
			break
		}

		var val string
		val, tag, _ = strings.Cut(tail, ",")

		switch keyword {
		case "enum":
			if val == "" {
				return builder, fmt.Errorf("%w: missing enum value in jsonschema tag", errors.ErrUnsupported)
			}
			enums = append(enums, val)
		default:
			return builder, fmt.Errorf("%w: unrecognized jsonschema tag %q", errors.ErrUnsupported, keyword)
		}
	}

	if len(enums) > 0 {
		builder.AddEnum(enums)
	}

	return builder, nil
}
