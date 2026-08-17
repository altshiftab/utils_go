package types

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"iter"
	"maps"
	"math"
	"net/url"
	"slices"
	"strings"
	"sync"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/json/schema/notes"
)

// Schema is a JSON schema.
// A JSON schema determines whether an instance is valid or not.
// Do not create values of this type directly.
// Instead, unmarshal from JSON or use a draft-specific Builder.
//
// If you have an existing Schema, you can edit the Parts list,
// but you must call [Schema.Finalize] afterward.
// When adding a new Part it will help to use [Vocabulary.Keywords];
// each supported JSON schema draft has a Vocabulary package variable.
// You can't add keywords that refer to other parts of the schema by name,
// such as $ref.
type Schema struct {
	// The different elements of this Schema.
	Parts []Part
}

// Clone returns a copy of a Schema.
func (s *Schema) Clone() *Schema {
	return &Schema{Parts: slices.Clone(s.Parts)}
}

// String returns a somewhat readable representation of a Schema.
// The format differs from JSON output, and also includes internal
// information not stored in JSON.
func (s *Schema) String() string {
	var sb strings.Builder
	sb.WriteString("Schema{")
	for i, part := range s.Parts {
		if i > 0 {
			sb.WriteString(", ")
		}
		val := part.Value
		if part.Keyword.Generated {
			// Don't try to print schemas of generated keywords.
			// They can cause infinite recursion.
			switch part.Keyword.ArgType { //nolint:exhaustive // The default case handles the remaining, schema-bearing part types.
			case ArgTypeBool, ArgTypeString, ArgTypeStrings, ArgTypeInt, ArgTypeFloat:
			default:
				val = PartString("<not printed>")
			}
		}
		fmt.Fprintf(&sb, "{%s %v}", part.Keyword.Name, val)
	}
	sb.WriteByte('}')
	return sb.String()
}

// LookupKeyword returns the value associated with a keyword in the schema.
// The bool result reports whether the keyword is present at all.
func (s *Schema) LookupKeyword(keyword string) (PartValue, bool) {
	for _, part := range s.Parts {
		if !part.Keyword.Generated && part.Keyword.Name == keyword {
			return part.Value, true
		}
	}
	return nil, false
}

// Finalize sorts the schema keywords into the order required for validation.
// Normally there is no need to call this explicitly.
// It will be called automatically by a Builder or by the JSON unmarshaler.
func (s *Schema) Finalize(v *Vocabulary) {
	slices.SortFunc(s.Parts, func(a, b Part) int {
		return v.Cmp(a.Keyword.Name, b.Keyword.Name)
	})
}

// Resolve resolves references across a schema and its subschemas.
// Normally there is no need to call this explicitly.
// It will be called automatically by the JSON unmarshaler.
func (s *Schema) Resolve(opts *ResolveOpts) error {
	var v *Vocabulary
	if opts != nil {
		v = opts.Vocabulary
	}

	if v == nil {
		for _, part := range s.Parts {
			if part.Keyword == &SchemaKeyword {
				v = LookupVocabulary(string(part.Value.(PartString)))
				if v == nil {
					return fmt.Errorf("%w: no registered vocabulary for schema %q when resolving", ErrInvalidSchema, part.Value.(PartString))
				}
				break
			}
		}
		if v == nil {
			return fmt.Errorf("%w: unknown schema vocabulary when resolving", ErrInvalidSchema)
		}
	}

	if opts == nil {
		opts = &ResolveOpts{
			Vocabulary: v,
			Loader:     loader,
		}
	}

	return v.Resolve(s, opts)
}

// Children returns an iterator over the immediate subschemas.
// The first iterator value is the name of the schema as used in a JSON pointer,
// the second is the schema itself.
func (s *Schema) Children() iter.Seq2[string, *Schema] {
	return func(yield func(string, *Schema) bool) {
		for _, part := range s.Parts {
			if part.Keyword.Generated {
				continue
			}

			switch part.Keyword.ArgType { //nolint:exhaustive // Only schema-bearing part types have children; the rest fall through.
			case ArgTypeSchema:
				if !yield(part.Keyword.Name, part.Value.(PartSchema).S) {
					return
				}

			case ArgTypeSchemas:
				for i, sub := range part.Value.(PartSchemas) {
					name := fmt.Sprintf("%s/%d", part.Keyword.Name, i)
					if !yield(name, sub) {
						return
					}
				}

			case ArgTypeMapSchema:
				// Sort for determinism.
				type keyVal struct {
					key string
					val *Schema
				}
				m := part.Value.(PartMapSchema)
				keyVals := make([]keyVal, 0, len(m))
				for k, v := range m {
					keyVals = append(keyVals, keyVal{k, v})
				}
				slices.SortFunc(keyVals, func(a, b keyVal) int {
					return strings.Compare(a.key, b.key)
				})
				for _, kv := range keyVals {
					name := part.Keyword.Name + "/" + kv.key
					if !yield(name, kv.val) {
						return
					}
				}

			case ArgTypeSchemaOrSchemas:
				pv := part.Value.(PartSchemaOrSchemas)
				if pv.Schema != nil {
					if !yield(part.Keyword.Name, pv.Schema) {
						return
					}
				} else {
					for i, sub := range pv.Schemas {
						name := fmt.Sprintf("%s/%d", part.Keyword.Name, i)
						if !yield(name, sub) {
							return
						}
					}
				}

			case ArgTypeMapArrayOrSchema:
				// Sort for determinism.
				type keyVal struct {
					key string
					val *Schema
				}
				m := part.Value.(PartMapArrayOrSchema)
				keyVals := make([]keyVal, 0, len(m))
				for k, v := range m {
					if v.Schema != nil {
						keyVals = append(keyVals, keyVal{k, v.Schema})
					}
				}
				slices.SortFunc(keyVals, func(a, b keyVal) int {
					return strings.Compare(a.key, b.key)
				})
				for _, kv := range keyVals {
					name := part.Keyword.Name + "/" + kv.key
					if !yield(name, kv.val) {
						return
					}
				}
			}
		}
	}
}

var (
	_ jsonv2.MarshalerTo     = (*Schema)(nil)
	_ jsonv2.UnmarshalerFrom = (*Schema)(nil)
)

// MarshalJSON marshals a [Schema] into JSON format.
// This implements [encoding/json.Marshaler].
func (s *Schema) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	if err := s.marshalSchema(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// marshalSchema marshals a [Schema] into JSON format,
// storing the results in buf.
func (s *Schema) marshalSchema(buf *bytes.Buffer) error {
	if isBoolSchema, isTrueSchema := s.isBoolSchema(); isBoolSchema {
		if isTrueSchema {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
		return nil
	}

	buf.WriteByte('{')

	first := true
	for _, part := range s.Parts {
		if part.Keyword.Generated {
			continue
		}

		if first {
			first = false
		} else {
			buf.WriteByte(',')
		}

		fmt.Fprintf(buf, "%s:", encodeString(part.Keyword.Name))

		switch v := part.Value.(type) {
		case PartBool:
			fmt.Fprintf(buf, "%t", v)
		case PartString:
			fmt.Fprintf(buf, "%s", encodeString(string(v)))
		case PartStrings:
			buf.WriteByte('[')
			for i, s := range v {
				if i > 0 {
					buf.WriteByte(',')
				}
				fmt.Fprintf(buf, "%s", encodeString(s))
			}
			buf.WriteByte(']')
		case PartStringOrStrings:
			if v.Strings == nil {
				fmt.Fprintf(buf, "%s", encodeString(v.String))
			} else {
				buf.WriteByte('[')
				for i, s := range v.Strings {
					if i > 0 {
						buf.WriteByte(',')
					}
					fmt.Fprintf(buf, "%s", encodeString(s))
				}
				buf.WriteByte(']')
			}
		case PartInt:
			fmt.Fprintf(buf, "%d", v)
		case PartFloat:
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return fmt.Errorf("%w: cannot marshal non-finite %q value %v", ErrInvalidSchema, part.Keyword.Name, float64(v))
			}
			if PartFloat(int64(v)) == v {
				fmt.Fprintf(buf, "%d", int64(v))
			} else if PartFloat(uint64(v)) == v {
				fmt.Fprintf(buf, "%d", uint64(v))
			} else {
				fmt.Fprintf(buf, "%g", v)
			}
		case PartSchema:
			if err := v.S.marshalSchema(buf); err != nil {
				return err
			}
		case PartSchemas:
			buf.WriteByte('[')
			for i, schema := range v {
				if i > 0 {
					buf.WriteByte(',')
				}
				if err := schema.marshalSchema(buf); err != nil {
					return err
				}
			}
			buf.WriteByte(']')
		case PartMapSchema:
			buf.WriteByte('{')
			// Sort the names for predictable results.
			names := slices.Collect(maps.Keys(v))
			slices.Sort(names)
			for i, name := range names {
				if i > 0 {
					buf.WriteByte(',')
				}
				fmt.Fprintf(buf, "%s:", encodeString(name))
				if err := v[name].marshalSchema(buf); err != nil {
					return err
				}
			}
			buf.WriteByte('}')
		case PartSchemaOrSchemas:
			if v.Schema != nil {
				if err := v.Schema.marshalSchema(buf); err != nil {
					return err
				}
			} else {
				buf.WriteByte('[')
				for i, schema := range v.Schemas {
					if i > 0 {
						buf.WriteByte(',')
					}
					if err := schema.marshalSchema(buf); err != nil {
						return err
					}
				}
				buf.WriteByte(']')
			}
		case PartMapArrayOrSchema:
			buf.WriteByte('{')
			// Sort the names for predictable results.
			names := slices.Collect(maps.Keys(v))
			slices.Sort(names)
			for i, name := range names {
				if i > 0 {
					buf.WriteByte(',')
				}
				fmt.Fprintf(buf, "%s:", encodeString(name))
				as := v[name]
				if as.Schema != nil {
					if err := as.Schema.marshalSchema(buf); err != nil {
						return err
					}
				} else {
					buf.WriteByte('[')
					for j, s := range as.Array {
						if j > 0 {
							buf.WriteByte(',')
						}
						fmt.Fprintf(buf, "%s", encodeString(s))
					}
					buf.WriteByte(']')
				}
			}
			buf.WriteByte('}')
		case PartAny:
			data, err := jsonv2.Marshal(v.V, jsonv2.Deterministic(true))
			if err != nil {
				return altshiftErrors.NewWithTrace(fmt.Errorf("json marshal %q value: %w", part.Keyword.Name, err), v.V)
			}
			buf.Write(data)
		default:
			return fmt.Errorf("%w: schema.MarshalJSON: unexpected type %T", ErrInvalidSchema, part.Value)
		}
	}

	buf.WriteByte('}')

	return nil
}

// isBoolSchema reports whether schema is a boolean schema,
// and reports whether it is the "true" schema.
func (s *Schema) isBoolSchema() (bool, bool) {
	isBoolSchema := false
	isTrueSchema := false
	for _, part := range s.Parts {
		if part.Keyword == &SchemaKeyword || part.Keyword.Generated {
			continue
		}
		if part.Keyword != &BoolKeyword {
			return false, false
		}
		isBoolSchema = true
		isTrueSchema = bool(part.Value.(PartBool))
	}
	return isBoolSchema, isTrueSchema
}

// encodeString returns the JSON encoding of s.
func encodeString(s string) []byte {
	data, err := jsonv2.Marshal(s)
	if err != nil {
		panic(fmt.Sprintf("json marshal failed, which should be impossible: %v", err))
	}
	return data
}

// MarshalJSONTo marshals a [Schema] into JSON format,
// writing the result to enc.
// This implements [jsonv2.MarshalerTo].
func (s *Schema) MarshalJSONTo(enc *jsontext.Encoder) error {
	data, err := s.MarshalJSON()
	if err != nil {
		return err
	}
	if err := enc.WriteValue(data); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("jsontext encoder write value: %w", err))
	}
	return nil
}

// buildTopFromJSON builds a [Schema] from JSON parsed into the
// empty interface value v. This assumes that this is the root schema.
func (s *Schema) buildTopFromJSON(schemaID string, uri *url.URL, v any) (*Vocabulary, error) {
	var version string
	sawSchemaKeyword := false
	if m, ok := v.(map[string]any); ok {
		if schemaVal, ok := m["$schema"]; ok {
			version, ok = schemaVal.(string)
			if !ok {
				return nil, fmt.Errorf("%w: $schema does not have a string value", ErrInvalidSchema)
			}
			sawSchemaKeyword = true
			s.Parts = append(s.Parts,
				Part{
					&SchemaKeyword,
					PartString(version),
				},
			)
			// Remove the keyword from a copy so that the
			// caller's map is not modified.
			m = maps.Clone(m)
			delete(m, "$schema")
		}
		v = m
	}

	if version == "" && schemaID != "" {
		version = schemaID
	}

	var vocabulary *Vocabulary
	if version == "" {
		vocabulary = DefaultVocabulary()
		if vocabulary == nil {
			return nil, fmt.Errorf("%w: JSON schema version not specified and there is no default", ErrInvalidSchema)
		}
	} else {
		vocabulary = LookupVocabulary(version)
		if vocabulary == nil {
			return nil, fmt.Errorf("%w: JSON schema version %q not recognized", ErrInvalidSchema, version)
		}
	}

	// Record the schema version so that a later Resolve
	// can find the vocabulary.
	if !sawSchemaKeyword {
		s.Parts = append(
			s.Parts,
			Part{
				&SchemaKeyword,
				PartString(vocabulary.Schema),
			},
		)
	}

	err := s.buildFromJSON(v, vocabulary)
	return vocabulary, err
}

// SchemaFromJSON builds a [Schema] from a JSON value that has
// already been parsed. This could be used as something like
//
//	var v any
//	if err := json.Unmarshal(data, &v); err != nil { ... }
//	s, err := schema.SchemaFromJSON(schemaID, uri, v)
//
// This can be useful in cases where it's not clear whether the
// JSON encoding contains a schema or not.
//
// The optional schemaID argument is something like [draft202012.SchemaID].
// The optional uri is where the schema was loaded from.
//
// It is normally necessary to call Resolve on the result.
func SchemaFromJSON(schemaID string, uri *url.URL, v any) (*Schema, error) {
	var s Schema
	if _, err := s.buildTopFromJSON(schemaID, uri, v); err != nil {
		return nil, err
	}
	return &s, nil
}

// buildFromJSON builds a [Schema] from JSON parsed into the
// empty interface value v.
func (s *Schema) buildFromJSON(v any, vocabulary *Vocabulary) error {
	switch v := v.(type) {
	case bool:
		s.Parts = append(s.Parts, Part{
			&BoolKeyword,
			PartBool(v),
		})

	case map[string]any:
		for keyword, val := range v {
			if err := s.addKeywordFromJSON(keyword, val, vocabulary); err != nil {
				return err
			}
		}
		s.Finalize(vocabulary)

	default:
		return fmt.Errorf("%w: unexpected type %T while JSON decoding schema", ErrInvalidSchema, v)
	}
	return nil
}

// addKeywordFromJSON adds a [Schema] keyword and value parsed from JSON.
func (s *Schema) addKeywordFromJSON(keyword string, val any, vocabulary *Vocabulary) error {
	if len(keyword) == 0 {
		return fmt.Errorf("%w: empty JSON keyword", ErrInvalidSchema)
	}

	sk, ok := vocabulary.Keywords[keyword]
	if !ok {
		// Unrecognized keywords are ignored.
		// They do not affect the validation result.
		s.Parts = append(s.Parts, Part{
			Keyword: &Keyword{
				Name:     keyword,
				ArgType:  ArgTypeAny,
				Validate: validateTrue,
			},
			Value: PartAny{val},
		})
		return nil
	}

	var spv PartValue
	switch sk.ArgType {
	case ArgTypeBool:
		b, ok := val.(bool)
		if !ok {
			return fmt.Errorf("%w: %q argument is type %T, want bool", ErrInvalidSchema, keyword, val)
		}
		spv = PartBool(b)
	case ArgTypeString:
		s, ok := val.(string)
		if !ok {
			return fmt.Errorf("%w: %q argument is type %T, want string", ErrInvalidSchema, keyword, val)
		}
		spv = PartString(s)
	case ArgTypeStrings:
		vals, ok := val.([]any)
		if !ok {
			return fmt.Errorf("%w: %q argument is type %T, want array of string", ErrInvalidSchema, keyword, val)
		}
		strs := make([]string, 0, len(vals))
		for i, v := range vals {
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("%w: %q argument item %d is %T, want string", ErrInvalidSchema, keyword, i, v)
			}
			strs = append(strs, s)
		}
		spv = PartStrings(strs)
	case ArgTypeStringOrStrings:
		s, ok := val.(string)
		if ok {
			spv = PartStringOrStrings{String: s}
		} else {
			vals, ok := val.([]any)
			if !ok {
				return fmt.Errorf("%w: %q argument is type %T, want string or array of string", ErrInvalidSchema, keyword, val)
			}
			strs := make([]string, 0, len(vals))
			for i, v := range vals {
				s, ok := v.(string)
				if !ok {
					return fmt.Errorf("%w: %q argument item %d is %T, want string", ErrInvalidSchema, keyword, i, v)
				}
				strs = append(strs, s)
			}
			spv = PartStringOrStrings{Strings: strs}
		}
	case ArgTypeInt:
		f, ok := val.(float64)
		if !ok {
			return fmt.Errorf("%w: %q argument is type %T, want integer", ErrInvalidSchema, keyword, val)
		}
		if f != math.Trunc(f) {
			return fmt.Errorf("%w: %q argument is non-integer, want integer", ErrInvalidSchema, keyword)
		}
		spv = PartInt(f)
	case ArgTypeFloat:
		f, ok := val.(float64)
		if !ok {
			return fmt.Errorf("%w: %q argument is type %T, want number", ErrInvalidSchema, keyword, val)
		}
		spv = PartFloat(f)
	case ArgTypeSchema:
		var s Schema
		if err := s.buildFromJSON(val, vocabulary); err != nil {
			return err
		}
		spv = PartSchema{&s}
	case ArgTypeSchemas:
		as, ok := val.([]any)
		if !ok {
			return fmt.Errorf("%w: %q argument is type %T, want array", ErrInvalidSchema, keyword, val)
		}
		schemas := make([]*Schema, 0, len(as))
		for _, a := range as {
			var s Schema
			if err := s.buildFromJSON(a, vocabulary); err != nil {
				return err
			}
			schemas = append(schemas, &s)
		}
		spv = PartSchemas(schemas)
	case ArgTypeMapSchema:
		jm, ok := val.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: %q argument is type %T, want object", ErrInvalidSchema, keyword, val)
		}
		nm := make(map[string]*Schema, len(jm))
		for k, v := range jm {
			var s Schema
			if err := s.buildFromJSON(v, vocabulary); err != nil {
				return err
			}
			nm[k] = &s
		}
		spv = PartMapSchema(nm)
	case ArgTypeSchemaOrSchemas:
		var (
			schema  *Schema
			schemas []*Schema
		)
		as, ok := val.([]any)
		if ok {
			schemas = make([]*Schema, 0, len(as))
			for _, a := range as {
				var s Schema
				if err := s.buildFromJSON(a, vocabulary); err != nil {
					return err
				}
				schemas = append(schemas, &s)
			}
		} else {
			var s Schema
			if err := s.buildFromJSON(val, vocabulary); err != nil {
				return err
			}
			schema = &s
		}
		spv = PartSchemaOrSchemas{
			Schema:  schema,
			Schemas: schemas,
		}
	case ArgTypeMapArrayOrSchema:
		jm, ok := val.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: %q argument is type %T, want object", ErrInvalidSchema, keyword, val)
		}
		nm := make(map[string]ArrayOrSchema, len(jm))
		for k, v := range jm {
			var as ArrayOrSchema
			switch v := v.(type) {
			case bool, map[string]any:
				var s Schema
				if err := s.buildFromJSON(v, vocabulary); err != nil {
					return err
				}
				as.Schema = &s
			case []any:
				strs := make([]string, 0, len(v))
				for i, v := range v {
					s, ok := v.(string)
					if !ok {
						return fmt.Errorf("%w: %q argument item %s:%d is %T, want string", ErrInvalidSchema, keyword, k, i, v)
					}
					strs = append(strs, s)
				}
				as.Array = strs
			default:
				return fmt.Errorf("%w: %q argument item %s is %T, want schema or array of strings", ErrInvalidSchema, keyword, k, v)
			}
			nm[k] = as
		}
		spv = PartMapArrayOrSchema(nm)
	case ArgTypeAny:
		spv = PartAny{val}
	default:
		panic("can't happen")
	}

	s.Parts = append(s.Parts, Part{
		Keyword: sk,
		Value:   spv,
	})
	return nil
}

// An instance may be an object read from JSON,
// with a Go type like map[string]any or []any.
// An instance may also be a Go struct or a pointer to a Go struct;
// in this case json tags on fields are used when matching field names.

// Validate reports whether instance satisfies the schema.
// If it does, this returns nil.
// If it does not, this returns a [*ValidateError] that collects
// the individual [*ValidationError] values.
// A non-nil error of a different type indicates some error
// during validation processing.
func (s *Schema) Validate(instance any) error {
	err := s.ValidateWithOpts(instance, &ValidateOpts{ValidateFormat: true})
	if err == nil {
		return nil
	}

	var collected []*ValidationError

	if validationErrors, ok := errors.AsType[*ValidationErrors](err); ok {
		collected = append(collected, validationErrors.Errs...)
	} else if validationError, ok := errors.AsType[*ValidationError](err); ok {
		collected = append(collected, validationError)
	}

	if len(collected) == 0 {
		return fmt.Errorf("schema validate: %w", err)
	}

	return NewValidateError(err, collected)
}

// ValidateOpts describes validation options.
// These are uncommon so we use a separate method for them.
type ValidateOpts struct {
	// Whether to modify the instance being validated by setting defaults.
	// If this is true, then defaults are applied when:
	//   - a "properties" keyword is applied to a map or a struct
	//   - a "prefixItems" keyword is applied to a slice or array
	//   - a "items" keyword with an array argument (pre draft2020-12)
	//     is applied to a slice or array.
	// In these cases, if the subschema has a "default" keyword,
	// and the value in question is the zero value of its type
	// (or, in the case of a map, is missing), then the instance
	// is modified to be set to the default.
	// Defaults are ignored for required properties,
	// as the user must supply them.
	//
	// This operation may panic if the instance can't be modified.
	//
	// The modification is made before validation;
	// if the default value is not permitted by the rest of
	// the schema, validation may fail.
	ApplyDefaults bool

	// Whether to validate the format keyword.
	// In order for this to be effective, the package
	// jsonschema/format must be blank imported;
	// by default the format keyword always matches.
	ValidateFormat bool
}

// ValidateWithOpts is like Validate but supports options.
func (s *Schema) ValidateWithOpts(instance any, opts *ValidateOpts) error {
	var versionData any
	state := &ValidationState{
		Root:        s,
		VersionData: &versionData,
		Opts:        opts,
	}
	state.RootState = state
	return s.ValidateSubSchema(instance, state)
}

// ValidateInPlaceSchema reports whether instance satisfies schema,
// where schema is a subschema that is evaluated in the same context
// as the parent schema.
func (s *Schema) ValidateInPlaceSchema(instance any, state *ValidationState) error {
	subState, err := state.Child()
	if err != nil {
		return err
	}
	subState.Schema = s

	var topErr error
	for i, p := range s.Parts {
		if p.Keyword.Validate == nil {
			continue
		}
		subState.Index = i
		if err := p.Keyword.Validate(p.Value, instance, subState); err != nil {
			// Prefix with the current keyword name only if the error lacks any location.
			if hasAnyLocation(err) {
				AddError(&topErr, err, "")
			} else {
				AddError(&topErr, err, p.Keyword.Name)
			}
		}
	}

	state.Notes.AddNotes(subState.Notes)

	return topErr
}

// ValidateSubSchema reports whether instance satisfies schema,
// where schema is a sub-schema of some larger validation request.
// This is like Validate but also accepts the current validation state.
func (s *Schema) ValidateSubSchema(instance any, state *ValidationState) error {
	subState, err := state.Child()
	if err != nil {
		return err
	}
	subState.Schema = s

	var topErr error
	for i, p := range s.Parts {
		if p.Keyword.Validate == nil {
			continue
		}
		subState.Index = i
		if err := p.Keyword.Validate(p.Value, instance, subState); err != nil {
			// Prefix with the current keyword name only if the error lacks any location.
			if hasAnyLocation(err) {
				AddError(&topErr, err, "")
			} else {
				AddError(&topErr, err, p.Keyword.Name)
			}
		}
	}
	return topErr
}

// hasAnyLocation reports whether err already has a populated keyword or instance location.
func hasAnyLocation(err error) bool {
	switch e := err.(type) { //nolint:errorlint // Validation errors are aggregated, not wrapped; direct type matching is deliberate.
	case *ValidationError:
		return e.KeywordLocation != "" || e.InstanceLocation != ""
	case *ValidationErrors:
		for _, ve := range e.Errs {
			if ve.KeywordLocation != "" || ve.InstanceLocation != "" {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// Keyword is a schema keyword.
type Keyword struct {
	// Name is the keyword, such as allOf, anyOf, and so forth.
	Name string

	// ArgType is the type of argument expected.
	ArgType ArgType

	// Validate is a function that checks whether the schema matches
	// the keyword. arg is the value from the schema, which is [Part.Value].
	// instance is the object to validate.
	//
	// The function returns an error if any.
	// A failure to validate will be type [*ValidationError]
	// or type [*ValidationErrors].
	// Any other error type indicates a problem with the schema itself,
	// not the instance.
	Validate func(arg PartValue, instance any, state *ValidationState) error

	// Generated is true if this keyword is not represented in JSON,
	// but is added to record additional information.
	// If this is true the keyword should be ignored by anything
	// that wants to treat the Schema as a JSON object.
	Generated bool
}

// Equal reports whether two keywords are equal.
// This is for the benefit of comparison packages such as
// github.com/altshiftab/utils_go/pkg/testing/cmp, which use it instead of
// comparing the Validate function values.
func (k1 Keyword) Equal(k2 Keyword) bool {
	return k1.Name == k2.Name && k1.ArgType == k2.ArgType && k1.Generated == k2.Generated
}

// Part is one part of a JSON schema.
// This is a keyword, such as "$id" or "properties",
// along with the value associated with that keyword in the schema.
type Part struct {
	Keyword *Keyword
	Value   PartValue
}

// MakePart builds a Part.
func MakePart(keyword *Keyword, value PartValue) Part {
	return Part{
		Keyword: keyword,
		Value:   value,
	}
}

// PartValue is the value of a JSON schema element.
// This is accessed via a type switch.
// The possible types are
//   - [PartBool]
//   - [PartString]
//   - [PartStrings]
//   - [PartStringOrStrings]
//   - [PartInt]
//   - [PartFloat]
//   - [PartSchema]
//   - [PartSchemas]
//   - [PartMapSchema]
//   - [PartSchemaOrSchemas]
//   - [PartMapArrayOrSchema]
//   - [PartAny]
type PartValue interface {
	partValue() // restrict to types defined in this package
}

// PartBool is a schema part value that is a bool.
// This is a compact representation of a JSON schema.
// A value of true is the schema that matches every value.
// A value of false is the schema that matches no values.
type PartBool bool

// PartString is a schema part value that is a string.
// For example, the schema keyword "pattern" has a string
// value that must be a regexp that must match the instance value.
type PartString string

// PartStrings is a schema part value that is a list of strings.
// For example, the schema keyword "required" takes a list of strings
// where each string is a property that the instance is required to have.
type PartStrings []string

// PartStringOrStrings is a schema part that is either a single string
// or a list of strings. This is basically just for the "type" keyword,
// which takes either a single type string or an array of type strings.
// If the Strings is not nil, the String field must be the empty string.
type PartStringOrStrings struct {
	String  string
	Strings []string
}

// PartInt is a schema part value that is an integer.
// For example, the schema keyword "minLength" specifies
// the minimum length of a string.
type PartInt int64

// PartFloat is a schema part value that is a floating-point number.
// For example, the schema keyword "maximum" specifies the maximum
// value of a number.
type PartFloat float64

// PartSchema is a schema part value that is a reference to a schema.
// For example, the schema keyword "not" refers to a schema;
// the instance matches if it does not match that schema.
type PartSchema struct {
	S *Schema
}

// PartSchemas is a schema part value that is a list of schemas.
// For example, the schema keyword "allOf" matches an instance
// if the instance matches each schema in the list.
type PartSchemas []*Schema

// PartMapSchema is a schema part value that is a map from strings to schemas.
// For example, the schema keyword "properties" has a mapping
// from field names to schemas, and matches an instance if the
// corresponding instance fields match the schemas.
type PartMapSchema map[string]*Schema

// PartSchemaOrSchemas is either a single schema (like [PartSchema])
// or a list of schemas (like [PartSchemas]). For example,
// the draft201909 keyword "items" takes either a single schema
// or a list of schemas. Exactly one of the fields will be nil.
type PartSchemaOrSchemas struct {
	Schema  *Schema
	Schemas []*Schema
}

// PartMapArrayOrSchema is a map from strings to elements,
// where each element is either an array of strings or a schema.
// This is used for the draft7 "dependencies" keyword.
type PartMapArrayOrSchema map[string]ArrayOrSchema

// ArrayOrSchema is the element type of the PartMapArrayOrSchema map.
// Exactly one of the fields will be nil.
type ArrayOrSchema struct {
	Array  []string // a zero-length slice is []string{}, not nil
	Schema *Schema
}

// PartAny is a schema part value that is an arbitrary type.
// For example, the schema keyword "$vocabulary" expects an
// object where each property is a URI.
// For example, the schema keyword "enum" expects an array,
// and matches an instance if the instance is equal to one of the
// elements in the array.
type PartAny struct {
	V any
}

func (PartBool) partValue()             {}
func (PartString) partValue()           {}
func (PartStrings) partValue()          {}
func (PartStringOrStrings) partValue()  {}
func (PartInt) partValue()              {}
func (PartFloat) partValue()            {}
func (PartSchema) partValue()           {}
func (PartSchemas) partValue()          {}
func (PartMapSchema) partValue()        {}
func (PartSchemaOrSchemas) partValue()  {}
func (PartMapArrayOrSchema) partValue() {}
func (PartAny) partValue()              {}

// ResolveOpts is options to use when resolving the schema.
// These are all optional.
type ResolveOpts struct {
	// The vocabulary to use.
	// This overrides anything recorded with the schema.
	Vocabulary *Vocabulary
	// URI of root of schema.
	// This is overridden by a $id keyword, if present.
	URI *url.URL
	// Load a remote reference, specifying the default schema.
	// This will be resolved by the resolver of the schema that
	// references it; no need for Loader to call (*Schema).Resolve.
	Loader func(schemaID string, uri *url.URL) (*Schema, error)
}

// SetLoader sets a function to call when resolving a $ref
// to an external schema. This is a global property,
// as there is no way to pass the desired value into the JSON decoder.
// Callers should use appropriate locking.
//
// Note that when unmarshaling user-written schemas,
// the loader function can be called with arbitrary URIs.
// It's probably unwise to simply call [net/http.Get] in all cases.
//
// To fully support JSON schema cross references, the loader should call
// [SchemaFromJSON]. The caller will handle calling [Schema.Resolve].
//
// This returns the old loader function.
// The default loader function is nil, which will produce an
// error for a $ref to an external schema.
func SetLoader(fn func(schemaID string, uri *url.URL) (*Schema, error)) func(string, *url.URL) (*Schema, error) {
	ret := loader
	loader = fn
	return ret
}

// loader is the default loader function.
var loader func(schemaID string, uri *url.URL) (*Schema, error)

// ValidationState is state we maintain while validating a schema.
// This does not apply to subschemas or parent schemas.
// This is exported for use by additional schema implementations.
// It is not expected to be used by code that just wants to validate a schema.
type ValidationState struct {
	// The root of the Schema being validated.
	Root *Schema
	// The ValidationState attached to the root Schema,
	// for global information.
	RootState *ValidationState
	// The Schema being validated.
	Schema *Schema
	// The index in schema.Parts of the keyword currently being validated.
	Index int
	// Current URI, from $id keyword.
	URI *url.URL
	// Notes created during validation.
	Notes notes.Notes
	// Depth of tree when validating. Used to avoid infinite recursion.
	Depth int
	// Validation options. Nil for the defaults.
	Opts *ValidateOpts
	// For use by version-specific code.
	VersionData *any

	// InstancePath holds the JSON Pointer tokens to the current location
	// within the instance being validated.
	InstancePath []string
}

// Child returns a new ValidationState that is a child of vs.
// This can be used to validate a subschema without changing
// the notes stored in vs.
func (vs *ValidationState) Child() (*ValidationState, error) {
	if vs.Depth > 1000 {
		return nil, fmt.Errorf("%w: recursion while validating schema too deep", ErrInvalidSchema)
	}

	ret := &ValidationState{
		Root:         vs.Root,
		RootState:    vs.RootState,
		Schema:       vs.Schema,
		Index:        vs.Index,
		URI:          vs.URI,
		Depth:        vs.Depth + 1,
		Opts:         vs.Opts,
		VersionData:  vs.VersionData,
		InstancePath: append([]string(nil), vs.InstancePath...),
	}
	return ret, nil
}

// PushInstanceToken appends a token to the instance path.
func (vs *ValidationState) PushInstanceToken(tok string) {
	vs.InstancePath = append(vs.InstancePath, tok)
}

// PopInstanceToken removes the last token from the instance path.
func (vs *ValidationState) PopInstanceToken() {
	if n := len(vs.InstancePath); n > 0 {
		vs.InstancePath = vs.InstancePath[:n-1]
	}
}

// InstancePointer returns the current instance location as a JSON Pointer
// string starting with '#'.
func (vs *ValidationState) InstancePointer() string {
	if len(vs.InstancePath) == 0 {
		return "#"
	}
	// Escape per RFC 6901
	b := make([]byte, 0, 2*len(vs.InstancePath))
	b = append(b, '#', '/')
	for i, t := range vs.InstancePath {
		if i > 0 {
			b = append(b, '/')
		}
		// Replace ~ with ~0, / with ~1
		for j := range len(t) {
			switch t[j] {
			case '~':
				b = append(b, '~', '0')
			case '/':
				b = append(b, '~', '1')
			default:
				b = append(b, t[j])
			}
		}
	}
	return string(b)
}

// EnsureInstanceLocation sets InstanceLocation on validation errors if empty.
func EnsureInstanceLocation(err error, ptr string) error {
	switch e := err.(type) { //nolint:errorlint // Validation errors are aggregated, not wrapped; direct type matching is deliberate.
	case *ValidationError:
		if e.InstanceLocation == "" || e.InstanceLocation == "#" {
			e.InstanceLocation = ptr
		}
		return e
	case *ValidationErrors:
		for _, ve := range e.Errs {
			if ve.InstanceLocation == "" || ve.InstanceLocation == "#" {
				ve.InstanceLocation = ptr
			}
		}
		return e
	default:
		return err
	}
}

// SchemaKeyword is a keyword to hold the schema version.
var SchemaKeyword = Keyword{
	Name:     "$schema",
	ArgType:  ArgTypeString,
	Validate: validateTrue,
}

// BoolKeyword is not a real keyword, but is used to represent the
// special schema values "true" and "false".
var BoolKeyword = Keyword{
	Name:     "$bool",
	ArgType:  ArgTypeBool,
	Validate: validateBool,
}

// validateTrue is a validator function that always succeeds.
func validateTrue(PartValue, any, *ValidationState) error {
	return nil
}

// validateBool handles the special $bool keyword,
// which does not actually appear in schema definitions.
func validateBool(arg PartValue, instance any, state *ValidationState) error {
	b := arg.(PartBool)
	if !b {
		return &ValidationError{
			Message: "false schema never matches",
		}
	}
	return nil
}

// Vocabulary is a vocabulary type: a list of known keywords.
// Each schema version defines an instance of this type.
type Vocabulary struct {
	// The name of this schema version, for messages.
	// Something like draft-2020-12.
	Name string
	// The URI that describes this schema version.
	// The value of the $schema keyword.
	// Something like "https://json-schema.org/draft/2020-12/schema".
	Schema string
	// The keywords of this schema version.
	Keywords map[string]*Keyword
	// A function that resolves references within a schema.
	Resolve func(*Schema, *ResolveOpts) error
	// The sorting function of this schema.
	// Used to sort the keywords of an instance of the schema.
	Cmp func(string, string) int
}

// A registry is a mapping from schema name to Vocabulary.
type registry struct {
	mu      sync.Mutex
	mapping map[string]*Vocabulary
	defval  *Vocabulary // default vocabulary
}

// add adds an item to the registry.
func (r *registry) add(s string, v *Vocabulary, def bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.mapping == nil {
		r.mapping = make(map[string]*Vocabulary)
	}
	if _, found := r.mapping[s]; found {
		panic(fmt.Sprintf("multiple attempts to add %q to registry", s))
	}
	r.mapping[s] = v
	if def {
		if r.defval != nil {
			panic("multiple default vocabularies")
		}
		r.defval = v
	}
}

// lookup returns an element from the registry,
// or nil if not present.
func (r *registry) lookup(s string) *Vocabulary {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mapping[s]
}

// def returns the default vocabulary,
// or nil if there isn't one.
func (r *registry) def() *Vocabulary {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.defval != nil {
		return r.defval
	}
	if len(r.mapping) == 1 {
		for _, v := range r.mapping {
			return v
		}
	}
	return nil
}

// setDef sets the default vocabulary.
func (r *registry) setDef(s string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, v := range r.mapping {
		if v.Name == s {
			r.defval = v
			return nil
		}
	}

	return fmt.Errorf("%w: setting default to %q failed: unknown schema name", ErrInvalidSchema, s)
}

// clear clears the registry, removing all entries.
func (r *registry) clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mapping = nil
	r.defval = nil
}

// reg is the global registry.
var reg registry

// RegisterVocabulary registers a vocabulary.
// The def argument is true for the default vocabulary.
// It's normally not necessary to call this;
// importing a JSON schema version package will register it.
func RegisterVocabulary(v *Vocabulary, def bool) {
	reg.add(v.Schema, v, def)
}

// LookupVocabulary returns a registered vocabulary, or nil if no vocabulary
// was registered under that name.
// It's normally not necessary to call this;
// instead use something like draft202012.Vocabulary.
func LookupVocabulary(s string) *Vocabulary {
	// For draft7 we can see
	// "http://json-schema.org/draft-07/schema#"
	s = strings.TrimSuffix(s, "#")
	return reg.lookup(s)
}

// DefaultVocabulary returns the default vocabulary, or nil if there isn't one.
func DefaultVocabulary() *Vocabulary {
	return reg.def()
}

// SetDefaultSchema sets the default schema.
// The argument should be something like "draft7" or "draft2020-12".
// This is a global property, as there is no way to pass the desired
// value into the JSON decoder. Callers should use appropriate locking.
// This is mainly for tests.
func SetDefaultSchema(s string) error {
	return reg.setDef(s)
}

// ClearVocabularies discards the vocabulary registry.
// This is for tests.
func ClearVocabularies() {
	reg.clear()
}
