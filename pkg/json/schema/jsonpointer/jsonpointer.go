// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package jsonpointer implements JSON pointers for the jsonschema package.
// This is not a fully general package.
package jsonpointer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/altshiftab/utils_go/pkg/json/schema/internal/argtype"
	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

// DerefSchema takes a JSON pointer and a root schema and returns
// the schema to which the pointer refers.
// The schemaID parameter is the default schema ID.
func DerefSchema(schemaID string, root *schema.Schema, pointer string) (*schema.Schema, error) {
	s := root
	if pointer == "" {
		return s, nil
	}
	pointer = strings.TrimPrefix(pointer, "/")
	toks := strings.Split(pointer, "/")
	for i := 0; i < len(toks); i++ {
		tok := decodeToken(toks[i])
		found := false
		for _, part := range s.Parts {
			if part.Keyword.Generated {
				continue
			}
			if part.Keyword.Name != tok {
				continue
			}

			switch part.Keyword.ArgType { //nolint:exhaustive // The default case rejects the remaining, non-dereferenceable part types.
			case schema.ArgTypeSchema:
				s = part.Value.(schema.PartSchema).S

			case schema.ArgTypeSchemas:
				i++
				if i >= len(toks) {
					return nil, fmt.Errorf("%w: when dereferencing pointer %q expected array index after %q", schema.ErrInvalidSchema, pointer, tok)
				}
				tok = decodeToken(toks[i])
				idx, err := strconv.Atoi(tok)
				if err != nil {
					return nil, fmt.Errorf("%w: when dereferencing pointer %q got token %q, expected array index", schema.ErrInvalidSchema, pointer, tok)
				}
				schemas := part.Value.(schema.PartSchemas)
				if idx < 0 || idx >= len(schemas) {
					return nil, fmt.Errorf("%w: when dereferencing pointer %q array index %d out of range (length %d)", schema.ErrInvalidSchema, pointer, idx, len(schemas))
				}
				s = schemas[idx]

			case schema.ArgTypeMapSchema:
				i++
				if i >= len(toks) {
					return nil, fmt.Errorf("%w: when dereferencing pointer %q expected map key after %q", schema.ErrInvalidSchema, pointer, tok)
				}
				tok = decodeToken(toks[i])
				m := part.Value.(schema.PartMapSchema)
				ms, ok := m[tok]
				if !ok {
					return nil, fmt.Errorf("%w: when dereferencing pointer %q map key %q not present", schema.ErrInvalidSchema, pointer, tok)
				}
				s = ms

			case schema.ArgTypeSchemaOrSchemas:
				pv := part.Value.(schema.PartSchemaOrSchemas)
				if pv.Schema != nil {
					s = pv.Schema
				} else {
					i++
					if i >= len(toks) {
						return nil, fmt.Errorf("%w: when dereferencing pointer %q expected array index after %q", schema.ErrInvalidSchema, pointer, tok)
					}
					tok = decodeToken(toks[i])
					idx, err := strconv.Atoi(tok)
					if err != nil {
						return nil, fmt.Errorf("%w: when dereferencing pointer %q got token %q, expected array index", schema.ErrInvalidSchema, pointer, tok)
					}
					if idx < 0 || idx >= len(pv.Schemas) {
						return nil, fmt.Errorf("%w: when dereferencing pointer %q array index %d out of range (length %d)", schema.ErrInvalidSchema, pointer, idx, len(pv.Schemas))
					}
					s = pv.Schemas[idx]
				}

			case schema.ArgTypeMapArrayOrSchema:
				i++
				if i >= len(toks) {
					return nil, fmt.Errorf("%w: when dereferencing pointer %q expected map key after %q", schema.ErrInvalidSchema, pointer, tok)
				}
				tok = decodeToken(toks[i])
				m := part.Value.(schema.PartMapArrayOrSchema)
				mv, ok := m[tok]
				if !ok {
					return nil, fmt.Errorf("%w: when dereferencing pointer %q map key %q not present", schema.ErrInvalidSchema, pointer, tok)
				}
				if mv.Schema == nil {
					return nil, fmt.Errorf("%w: when dereferencing pointer %q map key %q is not a schema", schema.ErrInvalidSchema, pointer, tok)
				}
				s = mv.Schema

			case schema.ArgTypeAny:
				pv := part.Value.(schema.PartAny).V
			resolveLoop:
				for {
					switch v := pv.(type) {
					case bool, map[string]any:
						var err error
						s, err = schema.SchemaFromJSON(schemaID, nil, v)
						if err != nil {
							return nil, fmt.Errorf("%w: when dereferencing pointer %q failed to resolve unrecognized schema: %w", schema.ErrInvalidSchema, pointer, err)
						}
						break resolveLoop

					case []any:
						i++
						if i >= len(toks) {
							return nil, fmt.Errorf("%w: when dereferencing pointer %q expected array index after %q", schema.ErrInvalidSchema, pointer, tok)
						}
						tok = decodeToken(toks[i])
						idx, err := strconv.Atoi(tok)
						if err != nil {
							return nil, fmt.Errorf("%w: when dereferencing pointer %q for token %q, expected array index", schema.ErrInvalidSchema, pointer, tok)
						}
						if idx < 0 || idx >= len(v) {
							return nil, fmt.Errorf("%w: when dereferencing pointer %q array index %d out of range (length %d)", schema.ErrInvalidSchema, pointer, idx, len(v))
						}
						pv = v[idx]

					default:
						return nil, fmt.Errorf("%w: when dereferencing pointer %q unexpected type %T", schema.ErrInvalidSchema, pointer, v)
					}
				}

			default:
				return nil, fmt.Errorf("%w: when dereferencing pointer %q unexpected part type %s", schema.ErrInvalidSchema, pointer, argtype.Name(part.Keyword.ArgType))
			}

			found = true
			break
		}
		if !found {
			return nil, fmt.Errorf("%w: when dereferencing pointer %q no schema element %q", schema.ErrInvalidSchema, pointer, tok)
		}
	}

	return s, nil
}

// decodeToken unmangles a token in a JSON pointer.
func decodeToken(tok string) string {
	tok = strings.ReplaceAll(tok, "~1", "/")
	return strings.ReplaceAll(tok, "~0", "~")
}
