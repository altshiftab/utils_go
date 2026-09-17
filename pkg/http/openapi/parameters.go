package openapi

import (
	"fmt"
	"go/ast"
	"reflect"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	queryTag "github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/query_extractor/tag"
	openapiTypes "github.com/altshiftab/utils_go/pkg/http/openapi/types"
	altshiftJsonTag "github.com/altshiftab/utils_go/pkg/json/types/tag"
	typeExportErrors "github.com/altshiftab/utils_go/pkg/type_export/errors"
)

const parameterInQuery = "query"

// boolParameterDescription says what a bare parameter means, which is not a thing a schema can
// express: the extractor reads a parameter present with no value as true.
const boolParameterDescription = "Present with no value means true."

// scalarSchema describes one value the query extractor accepts, or reports the kinds it does not.
//
// The mapping is the extractor's, not the JSON exporter's. A []byte parameter is one string taken
// as it stands rather than base64, a pointer is refused outright rather than made nullable, and
// nothing nests: a query is a flat list of strings, and a schema saying otherwise would describe a
// request the server will not accept.
func scalarSchema(reflectType reflect.Type) (map[string]any, error) {
	//exhaustive:ignore
	switch kind := reflectType.Kind(); kind {
	case reflect.String:
		return map[string]any{schemaKeyType: schemaTypeString}, nil
	case reflect.Bool:
		return map[string]any{schemaKeyType: schemaTypeBoolean}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{schemaKeyType: schemaTypeInteger}, nil
	case reflect.Float32, reflect.Float64:
		return map[string]any{schemaKeyType: schemaTypeNumber}, nil
	default:
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %s", typeExportErrors.ErrUnsupportedKind, kind),
			kind,
		)
	}
}

// parameterFormat carries a query tag's format across to the schema. The vocabularies differ in one
// name: the extractor calls a URL "url", JSON Schema calls it "uri".
func parameterFormat(format string) string {
	switch strings.TrimSpace(format) {
	case "":
		return ""
	case "url":
		return "uri"
	default:
		return strings.TrimSpace(format)
	}
}

// fieldSchema describes one field of a query struct.
func fieldSchema(fieldType reflect.Type) (map[string]any, error) {
	//exhaustive:ignore
	switch fieldType.Kind() {
	case reflect.Pointer:
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: pointer", typeExportErrors.ErrUnsupportedKind),
			fieldType,
		)
	case reflect.Slice:
		// A []byte parameter is one string, set from the value as it stands.
		if fieldType.Elem().Kind() == reflect.Uint8 {
			return map[string]any{schemaKeyType: schemaTypeString}, nil
		}

		itemSchema, err := scalarSchema(fieldType.Elem())
		if err != nil {
			return nil, altshiftErrors.New(fmt.Errorf("scalar schema (items): %w", err), fieldType.Elem())
		}

		return map[string]any{schemaKeyType: schemaTypeArray, schemaKeyItems: itemSchema}, nil
	case reflect.Array:
		itemSchema, err := scalarSchema(fieldType.Elem())
		if err != nil {
			return nil, altshiftErrors.New(fmt.Errorf("scalar schema (items): %w", err), fieldType.Elem())
		}

		// A fixed-size array is exactly that many values, no more and no fewer.
		return map[string]any{
			schemaKeyType:     schemaTypeArray,
			schemaKeyItems:    itemSchema,
			schemaKeyMinItems: fieldType.Len(),
			schemaKeyMaxItems: fieldType.Len(),
		}, nil
	default:
		return scalarSchema(fieldType)
	}
}

// MakeParameters describes a query struct as the query parameters an operation takes.
//
// The walk is the query extractor's own, rather than the JSON schema exporter's, because the two
// accept different things: the extractor reads the query tag where the exporter reads the
// jsonschema tag, refuses pointers where the exporter makes them nullable, and imposes none of the
// minimum length the exporter puts on every string. A parameter list built the other way would
// document a request the server would refuse and refuse one it accepts.
//
// A nil or empty-interface type has no parameters, which is not an error: most endpoints take none.
func MakeParameters(reflectType reflect.Type) ([]*openapiTypes.Parameter, error) {
	if reflectType == nil {
		return nil, nil
	}

	for reflectType.Kind() == reflect.Pointer {
		reflectType = reflectType.Elem()
	}

	if reflectType.Kind() == reflect.Interface {
		return nil, nil
	}

	if kind := reflectType.Kind(); kind != reflect.Struct {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: query input is not a struct: %s", typeExportErrors.ErrUnsupportedKind, kind),
			kind,
		)
	}

	var parameters []*openapiTypes.Parameter

	for i := range reflectType.NumField() {
		field := reflectType.Field(i)

		identifier := field.Name
		if len(identifier) == 0 || !ast.IsExported(identifier) {
			continue
		}

		optional := false
		var format string

		// The query tag decides, and the json tag answers only where there is no query tag -- which
		// is what the extractor does, down to the json tag carrying no format.
		if tag := queryTag.New(field.Tag.Get("query")); tag != nil {
			if tag.Skip {
				continue
			}
			if name := tag.Name; name != "" {
				identifier = name
			}
			optional = tag.OmitEmpty || tag.OmitZero
			format = tag.Format
		} else if tag := altshiftJsonTag.New(field.Tag.Get("json")); tag != nil {
			if tag.Skip {
				continue
			}
			if name := tag.Name; name != "" {
				identifier = name
			}
			optional = tag.OmitEmpty || tag.OmitZero
		}

		schema, err := fieldSchema(field.Type)
		if err != nil {
			return nil, altshiftErrors.New(
				fmt.Errorf("field schema (%s): %w", identifier, err),
				identifier,
				field.Type,
			)
		}

		if parameterFormat := parameterFormat(format); parameterFormat != "" {
			schema[schemaKeyFormat] = parameterFormat
		}

		parameter := &openapiTypes.Parameter{
			Name:     identifier,
			In:       parameterInQuery,
			Required: !optional,
			Schema:   schema,
		}

		if schema[schemaKeyType] == schemaTypeBoolean {
			parameter.Description = boolParameterDescription
		}

		parameters = append(parameters, parameter)
	}

	return parameters, nil
}
