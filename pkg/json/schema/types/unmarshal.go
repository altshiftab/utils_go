// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// UnmarshalJSON decodes the JSON representation of a [Schema].
// This implements [encoding/json.Unmarshaler].
func (s *Schema) UnmarshalJSON(data []byte) error {
	dec := jsontext.NewDecoder(bytes.NewReader(data), jsontext.AllowDuplicateNames(false))
	if err := s.UnmarshalJSONFrom(dec); err != nil {
		return err
	}

	// The input must consist of a single JSON value.
	if _, err := dec.ReadToken(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: unexpected data after JSON schema", ErrInvalidSchema)
	}
	return nil
}

// UnmarshalJSONFrom decodes the JSON representation of a [Schema]
// read from dec. This implements [jsonv2.UnmarshalerFrom].
//
// The schema is decoded directly from the JSON text,
// without building an intermediate representation.
func (s *Schema) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	s.Parts = s.Parts[:0:0]

	vocabulary, err := s.decodeTop(dec)
	if err != nil {
		return err
	}

	ropts := &ResolveOpts{
		Vocabulary: vocabulary,
		Loader:     loader,
	}
	return s.Resolve(ropts)
}

// decodeTop decodes the root of a schema from dec.
// The root determines the vocabulary: the value of a "$schema" member,
// which may appear anywhere in the object, or the default vocabulary.
func (s *Schema) decodeTop(dec *jsontext.Decoder) (*Vocabulary, error) {
	switch dec.PeekKind() { //nolint:exhaustive // The default case rejects the remaining kinds.
	case 't', 'f':
		vocabulary := DefaultVocabulary()
		if vocabulary == nil {
			return nil, fmt.Errorf("%w: JSON schema version not specified and there is no default", ErrInvalidSchema)
		}
		tok, err := dec.ReadToken()
		if err != nil {
			return nil, motmedelErrors.NewWithTrace(fmt.Errorf("jsontext decoder read token: %w", err))
		}
		s.Parts = append(s.Parts,
			Part{&SchemaKeyword, PartString(vocabulary.Schema)},
			Part{&BoolKeyword, PartBool(tok.Bool())},
		)
		return vocabulary, nil

	case '{':
		// Handled below.

	default:
		return nil, fmt.Errorf("%w: unexpected JSON %s while decoding schema", ErrInvalidSchema, kindName(dec.PeekKind()))
	}

	// The vocabulary needed to interpret the keywords is not known
	// until the "$schema" member has been seen, so buffer the raw
	// member values first.
	if _, err := dec.ReadToken(); err != nil {
		return nil, motmedelErrors.NewWithTrace(fmt.Errorf("jsontext decoder read token: %w", err))
	}

	var (
		names   []string
		raws    []jsontext.Value
		version string
	)
	seen := make(map[string]bool)
	for dec.PeekKind() != '}' {
		nameToken, err := dec.ReadToken()
		if err != nil {
			return nil, motmedelErrors.NewWithTrace(fmt.Errorf("jsontext decoder read token: %w", err))
		}
		name := nameToken.String()

		// The decoder may be configured to permit duplicate names,
		// but a schema with duplicate keywords is an authoring error.
		if seen[name] {
			return nil, fmt.Errorf("%w: duplicate schema keyword %q", ErrInvalidSchema, name)
		}
		seen[name] = true

		raw, err := dec.ReadValue()
		if err != nil {
			return nil, motmedelErrors.NewWithTrace(fmt.Errorf("jsontext decoder read value: %w", err))
		}

		if name == SchemaKeyword.Name {
			if err := jsonv2.Unmarshal(raw, &version); err != nil {
				return nil, fmt.Errorf("%w: $schema does not have a string value", ErrInvalidSchema)
			}
		}

		names = append(names, name)
		// The value returned by ReadValue is only valid until the
		// next read, so keep a copy.
		raws = append(raws, append(jsontext.Value(nil), raw...))
	}
	if _, err := dec.ReadToken(); err != nil {
		return nil, motmedelErrors.NewWithTrace(fmt.Errorf("jsontext decoder read token: %w", err))
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
	schemaVersion := version
	if schemaVersion == "" {
		schemaVersion = vocabulary.Schema
	}
	s.Parts = append(s.Parts,
		Part{
			&SchemaKeyword,
			PartString(schemaVersion),
		},
	)

	for i, name := range names {
		if name == SchemaKeyword.Name {
			continue
		}
		valueDecoder := jsontext.NewDecoder(bytes.NewReader(raws[i]), jsontext.AllowDuplicateNames(false)) //nolint:nilaway // ReadValue returns a non-nil value when err is nil, and bytes.NewReader accepts nil regardless.
		if err := s.addKeywordFromDecoder(name, valueDecoder, vocabulary); err != nil {
			return nil, err
		}
	}
	s.Finalize(vocabulary)

	return vocabulary, nil
}

// decodeSub decodes a subschema from dec.
// Unlike the root, a subschema uses the vocabulary of its parent,
// so it can be decoded in a single streaming pass.
func (s *Schema) decodeSub(dec *jsontext.Decoder, vocabulary *Vocabulary) error {
	switch dec.PeekKind() { //nolint:exhaustive // The default case rejects the remaining kinds.
	case 't', 'f':
		tok, err := dec.ReadToken()
		if err != nil {
			return motmedelErrors.NewWithTrace(fmt.Errorf("jsontext decoder read token: %w", err))
		}
		s.Parts = append(s.Parts, Part{
			&BoolKeyword,
			PartBool(tok.Bool()),
		})
		return nil

	case '{':
		if _, err := dec.ReadToken(); err != nil {
			return motmedelErrors.NewWithTrace(fmt.Errorf("jsontext decoder read token: %w", err))
		}
		seen := make(map[string]bool)
		for dec.PeekKind() != '}' {
			nameToken, err := dec.ReadToken()
			if err != nil {
				return motmedelErrors.NewWithTrace(fmt.Errorf("jsontext decoder read token: %w", err))
			}
			name := nameToken.String()
			if seen[name] {
				return fmt.Errorf("%w: duplicate schema keyword %q", ErrInvalidSchema, name)
			}
			seen[name] = true
			if err := s.addKeywordFromDecoder(name, dec, vocabulary); err != nil {
				return err
			}
		}
		if _, err := dec.ReadToken(); err != nil {
			return motmedelErrors.NewWithTrace(fmt.Errorf("jsontext decoder read token: %w", err))
		}
		s.Finalize(vocabulary)
		return nil

	default:
		return fmt.Errorf("%w: unexpected JSON %s while decoding schema", ErrInvalidSchema, kindName(dec.PeekKind()))
	}
}

// addKeywordFromDecoder decodes the value of a single schema keyword
// from dec and appends the resulting Part.
func (s *Schema) addKeywordFromDecoder(keyword string, dec *jsontext.Decoder, vocabulary *Vocabulary) error {
	if len(keyword) == 0 {
		return fmt.Errorf("%w: empty JSON keyword", ErrInvalidSchema)
	}

	sk, ok := vocabulary.Keywords[keyword]
	if !ok {
		// Unrecognized keywords are ignored.
		// They do not affect the validation result.
		var v any
		if err := decodeRaw(dec, keyword, "value", &v); err != nil {
			return err
		}
		s.Parts = append(s.Parts, Part{
			Keyword: &Keyword{
				Name:     keyword,
				ArgType:  ArgTypeAny,
				Validate: validateTrue,
			},
			Value: PartAny{V: v},
		})
		return nil
	}

	var spv PartValue
	switch sk.ArgType {
	case ArgTypeBool:
		var b bool
		if err := decodeRaw(dec, keyword, "bool", &b); err != nil {
			return err
		}
		spv = PartBool(b)
	case ArgTypeString:
		var str string
		if err := decodeRaw(dec, keyword, "string", &str); err != nil {
			return err
		}
		spv = PartString(str)
	case ArgTypeStrings:
		var strs []string
		if err := decodeRaw(dec, keyword, "array of string", &strs); err != nil {
			return err
		}
		spv = PartStrings(strs)
	case ArgTypeStringOrStrings:
		switch dec.PeekKind() { //nolint:exhaustive // The default case rejects the remaining kinds.
		case '"':
			var str string
			if err := decodeRaw(dec, keyword, "string", &str); err != nil {
				return err
			}
			spv = PartStringOrStrings{String: str}
		case '[':
			var strs []string
			if err := decodeRaw(dec, keyword, "array of string", &strs); err != nil {
				return err
			}
			spv = PartStringOrStrings{Strings: strs}
		default:
			return fmt.Errorf("%w: %q argument is %s, want string or array of string", ErrInvalidSchema, keyword, kindName(dec.PeekKind()))
		}
	case ArgTypeInt:
		var f float64
		if err := decodeRaw(dec, keyword, "integer", &f); err != nil {
			return err
		}
		if f != math.Trunc(f) {
			return fmt.Errorf("%w: %q argument is non-integer, want integer", ErrInvalidSchema, keyword)
		}
		spv = PartInt(f)
	case ArgTypeFloat:
		var f float64
		if err := decodeRaw(dec, keyword, "number", &f); err != nil {
			return err
		}
		spv = PartFloat(f)
	case ArgTypeSchema:
		var sub Schema
		if err := sub.decodeSub(dec, vocabulary); err != nil {
			return err
		}
		spv = PartSchema{S: &sub}
	case ArgTypeSchemas:
		schemas, err := decodeSchemaArray(dec, keyword, vocabulary)
		if err != nil {
			return err
		}
		spv = PartSchemas(schemas)
	case ArgTypeMapSchema:
		if dec.PeekKind() != '{' {
			return fmt.Errorf("%w: %q argument is %s, want object", ErrInvalidSchema, keyword, kindName(dec.PeekKind()))
		}
		if _, err := dec.ReadToken(); err != nil {
			return motmedelErrors.NewWithTrace(fmt.Errorf("jsontext decoder read token: %w", err))
		}
		nm := make(map[string]*Schema)
		for dec.PeekKind() != '}' {
			nameToken, err := dec.ReadToken()
			if err != nil {
				return motmedelErrors.NewWithTrace(fmt.Errorf("jsontext decoder read token: %w", err))
			}
			// The token is voided by the next decoder call.
			name := nameToken.String()
			if _, ok := nm[name]; ok {
				return fmt.Errorf("%w: %q argument has duplicate member %q", ErrInvalidSchema, keyword, name)
			}
			var sub Schema
			if err := sub.decodeSub(dec, vocabulary); err != nil {
				return err
			}
			nm[name] = &sub
		}
		if _, err := dec.ReadToken(); err != nil {
			return motmedelErrors.NewWithTrace(fmt.Errorf("jsontext decoder read token: %w", err))
		}
		spv = PartMapSchema(nm)
	case ArgTypeSchemaOrSchemas:
		if dec.PeekKind() == '[' {
			schemas, err := decodeSchemaArray(dec, keyword, vocabulary)
			if err != nil {
				return err
			}
			spv = PartSchemaOrSchemas{Schemas: schemas}
		} else {
			var sub Schema
			if err := sub.decodeSub(dec, vocabulary); err != nil {
				return err
			}
			spv = PartSchemaOrSchemas{Schema: &sub}
		}
	case ArgTypeMapArrayOrSchema:
		if dec.PeekKind() != '{' {
			return fmt.Errorf("%w: %q argument is %s, want object", ErrInvalidSchema, keyword, kindName(dec.PeekKind()))
		}
		if _, err := dec.ReadToken(); err != nil {
			return motmedelErrors.NewWithTrace(fmt.Errorf("jsontext decoder read token: %w", err))
		}
		nm := make(map[string]ArrayOrSchema)
		for dec.PeekKind() != '}' {
			nameToken, err := dec.ReadToken()
			if err != nil {
				return motmedelErrors.NewWithTrace(fmt.Errorf("jsontext decoder read token: %w", err))
			}
			name := nameToken.String()
			if _, ok := nm[name]; ok {
				return fmt.Errorf("%w: %q argument has duplicate member %q", ErrInvalidSchema, keyword, name)
			}

			var as ArrayOrSchema
			switch dec.PeekKind() { //nolint:exhaustive // The default case rejects the remaining kinds.
			case 't', 'f', '{':
				var sub Schema
				if err := sub.decodeSub(dec, vocabulary); err != nil {
					return err
				}
				as.Schema = &sub
			case '[':
				strs := []string{}
				if err := decodeRaw(dec, keyword, "array of string", &strs); err != nil {
					return err
				}
				as.Array = strs
			default:
				return fmt.Errorf("%w: %q argument item %s is %s, want schema or array of strings", ErrInvalidSchema, keyword, name, kindName(dec.PeekKind()))
			}
			nm[name] = as
		}
		if _, err := dec.ReadToken(); err != nil {
			return motmedelErrors.NewWithTrace(fmt.Errorf("jsontext decoder read token: %w", err))
		}
		spv = PartMapArrayOrSchema(nm)
	case ArgTypeAny:
		var v any
		if err := decodeRaw(dec, keyword, "value", &v); err != nil {
			return err
		}
		spv = PartAny{V: v}
	default:
		panic("can't happen")
	}

	s.Parts = append(s.Parts, Part{
		Keyword: sk,
		Value:   spv,
	})
	return nil
}

// decodeSchemaArray decodes a JSON array of subschemas from dec.
func decodeSchemaArray(dec *jsontext.Decoder, keyword string, vocabulary *Vocabulary) ([]*Schema, error) {
	if dec.PeekKind() != '[' {
		return nil, fmt.Errorf("%w: %q argument is %s, want array", ErrInvalidSchema, keyword, kindName(dec.PeekKind()))
	}
	if _, err := dec.ReadToken(); err != nil {
		return nil, motmedelErrors.NewWithTrace(fmt.Errorf("jsontext decoder read token: %w", err))
	}
	var schemas []*Schema
	for dec.PeekKind() != ']' {
		var sub Schema
		if err := sub.decodeSub(dec, vocabulary); err != nil {
			return nil, err
		}
		schemas = append(schemas, &sub)
	}
	if _, err := dec.ReadToken(); err != nil {
		return nil, motmedelErrors.NewWithTrace(fmt.Errorf("jsontext decoder read token: %w", err))
	}
	return schemas, nil
}

// decodeRaw decodes the next JSON value from dec into target,
// reporting a keyword argument error on a type mismatch.
func decodeRaw(dec *jsontext.Decoder, keyword, want string, target any) error {
	kind := dec.PeekKind()
	raw, err := dec.ReadValue()
	if err != nil {
		return motmedelErrors.NewWithTrace(fmt.Errorf("jsontext decoder read value: %w", err))
	}
	if err := jsonv2.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("%w: %q argument is %s, want %s", ErrInvalidSchema, keyword, kindName(kind), want)
	}
	return nil
}

// kindName returns a human-readable name for a JSON kind.
func kindName(kind jsontext.Kind) string {
	switch kind { //nolint:exhaustive // The default case names the remaining kinds generically.
	case 'n':
		return "null"
	case 't', 'f':
		return "bool"
	case '"':
		return "string"
	case '0':
		return "number"
	case '{':
		return "object"
	case '[':
		return "array"
	default:
		return fmt.Sprintf("kind %q", byte(kind))
	}
}
