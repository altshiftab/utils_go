package types

import (
	"encoding/json/v2"
	"fmt"
	"reflect"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftJsonTag "github.com/altshiftab/utils_go/pkg/json/types/tag"
	altshiftReflect "github.com/altshiftab/utils_go/pkg/reflect"
	typeExportErrors "github.com/altshiftab/utils_go/pkg/type_export/errors"
	"github.com/altshiftab/utils_go/pkg/type_export/jsonschema/types/tag"
	typeExportContext "github.com/altshiftab/utils_go/pkg/type_export/types/context"
	"github.com/altshiftab/utils_go/pkg/type_export/types/type_declaration"
	"github.com/altshiftab/utils_go/pkg/utils"
)

type Context struct {
	*typeExportContext.Context
}

// JSON Schema type names.
const (
	schemaTypeString  = "string"
	schemaTypeInteger = "integer"
	schemaTypeNumber  = "number"
	schemaTypeBoolean = "boolean"
	schemaTypeArray   = "array"
	schemaTypeObject  = "object"
	schemaTypeNull    = "null"
)

func isTime(t reflect.Type) bool {
	return t.Name() == "Time" && t.PkgPath() == "time"
}

// GetJSONSchemaType returns a JSON Schema fragment describing the provided type.
func (c *Context) GetJSONSchemaType(reflectType reflect.Type) (map[string]any, error) {
	reflectType = altshiftReflect.RemoveIndirection(reflectType)

	//exhaustive:ignore
	switch kind := reflectType.Kind(); kind {
	case reflect.Struct:
		if isTime(reflectType) {
			return map[string]any{"type": schemaTypeString, "format": "date-time"}, nil
		}

		// Reference another interface via local $defs
		typeDeclaration, ok := c.TypeDeclarations[reflectType]
		if ok {
			if iface, ok2 := typeDeclaration.(*type_declaration.InterfaceDeclaration); ok2 {
				return map[string]any{"$ref": "#/$defs/" + iface.QualifiedName()}, nil
			}
		}
		return nil, altshiftErrors.NewWithTrace(typeExportErrors.ErrUnsupportedKind, kind)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": schemaTypeInteger}, nil
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": schemaTypeNumber}, nil
	case reflect.String:
		return map[string]any{"type": schemaTypeString}, nil
	case reflect.Bool:
		return map[string]any{"type": schemaTypeBoolean}, nil
	case reflect.Slice, reflect.Array:
		// Special case: []byte -> base64 string
		elem := altshiftReflect.RemoveIndirection(reflectType.Elem())
		if elem.Kind() == reflect.Uint8 {
			return map[string]any{"type": schemaTypeString, "contentEncoding": "base64"}, nil
		}
		itemSchema, err := c.GetJSONSchemaType(elem)
		if err != nil {
			return nil, altshiftErrors.New(fmt.Errorf("get json schema type (items): %w", err), elem)
		}
		return map[string]any{"type": schemaTypeArray, "items": itemSchema}, nil
	case reflect.Map:
		// JSON object with additionalProperties as value schema
		value := altshiftReflect.RemoveIndirection(reflectType.Elem())
		valueSchema, err := c.GetJSONSchemaType(value)
		if err != nil {
			return nil, altshiftErrors.New(fmt.Errorf("get json schema type (map value): %w", err), value)
		}
		return map[string]any{"type": schemaTypeObject, "additionalProperties": valueSchema}, nil
	case reflect.Interface:
		return map[string]any{}, nil
	case reflect.Pointer:
		return c.GetJSONSchemaType(reflectType.Elem())
	default:
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %T", typeExportErrors.ErrUnsupportedKind, kind), kind,
		)
	}
}

// makeNullable extends the schema so that JSON null is also a valid value.
// For schemas with a plain "type" string, it widens to a [type, "null"] tuple.
// For schemas using "$ref" (or any other shape), it wraps in anyOf with a null
// alternative so the reference is preserved.
func makeNullable(propertySchema map[string]any) map[string]any {
	if _, hasRef := propertySchema["$ref"]; hasRef {
		return map[string]any{
			"anyOf": []any{
				propertySchema,
				map[string]any{"type": schemaTypeNull},
			},
		}
	}

	switch t := propertySchema["type"].(type) {
	case string:
		if t != schemaTypeNull {
			propertySchema["type"] = []any{t, schemaTypeNull}
		}
	case []any:
		hasNull := false
		for _, x := range t {
			if x == schemaTypeNull {
				hasNull = true
				break
			}
		}
		if !hasNull {
			propertySchema["type"] = append(t, schemaTypeNull)
		}
	default:
		return map[string]any{
			"anyOf": []any{
				propertySchema,
				map[string]any{"type": schemaTypeNull},
			},
		}
	}

	return propertySchema
}

// additionalPropertiesMarkerName is the blank field a struct says through what holds for the object
// rather than for any one property. It is no property itself, being unexported, so it is read from
// the type rather than from the declaration's properties.
const additionalPropertiesMarkerName = "_"

// additionalPropertiesFromType reads what a struct says about members its properties do not name,
// through the jsonschema tag of its blank field: `jsonschema:"-,additionalProperties:true"`. A
// struct saying nothing is closed, which is what a schema derived from a Go type should be.
func additionalPropertiesFromType(structType reflect.Type) (any, error) {
	if structType == nil {
		return false, nil
	}

	markerField, ok := structType.FieldByName(additionalPropertiesMarkerName)
	if !ok {
		return false, nil
	}

	rawMarkerTag := markerField.Tag.Get("jsonschema")
	markerTag, err := tag.New(rawMarkerTag)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("jsonschema tag new: %w", err), rawMarkerTag)
	}
	if markerTag == nil || markerTag.AdditionalProperties == nil {
		return false, nil
	}

	return *markerTag.AdditionalProperties, nil
}

// buildInterfaceSchema builds the object schema for a given interface declaration.
func (c *Context) buildInterfaceSchema(interfaceDeclaration *type_declaration.InterfaceDeclaration) (map[string]any, error) {
	schemaMap := map[string]any{
		"type": schemaTypeObject,
	}

	properties := map[string]any{}
	var requiredProperties []string

	// An object is closed to what its properties do not name, so that a member a caller misspells
	// is refused rather than ignored. A struct says otherwise through the jsonschema tag of a
	// blank field, which is what a body whose members someone else decides needs: a schema
	// rejecting the batch would lose what it was sent to carry.
	additionalProps, err := additionalPropertiesFromType(interfaceDeclaration.Type)
	if err != nil {
		return nil, fmt.Errorf("additional properties from type: %w", err)
	}

	for _, property := range interfaceDeclaration.Properties {
		if property == nil {
			continue
		}

		field := property.Field
		if field == nil {
			return nil, altshiftErrors.NewWithTrace(nil_error.New("property field"), property)
		}

		identifier := property.Identifier
		isOptional := property.Optional

		rawJsonSchemaTag := field.Tag.Get("jsonschema")
		jsonschemaTag, err := tag.New(rawJsonSchemaTag)
		if err != nil {
			return nil, altshiftErrors.New(fmt.Errorf("jsonschema tag new: %w", err), rawJsonSchemaTag)
		}

		if jsonschemaTag != nil {
			if jsonschemaTag.Skip {
				continue
			}

			if name := jsonschemaTag.Name; name != "" {
				identifier = name
			}

			if jsonschemaTag.Optional {
				isOptional = true
			}
		} else {
			// As a fallback, use the `json` tag.
			jsonTagRaw := field.Tag.Get("json")
			jsonTag := altshiftJsonTag.New(jsonTagRaw)
			if jsonTag != nil {
				if jsonTag.Skip {
					continue
				}

				if name := jsonTag.Name; name != "" {
					identifier = name
				}

				isOptional = isOptional || jsonTag.OmitEmpty || jsonTag.OmitZero
			}
		}

		fieldType := field.Type
		isNullable := fieldType.Kind() == reflect.Pointer

		propertySchema, err := c.GetJSONSchemaType(fieldType)
		if err != nil {
			return nil, altshiftErrors.New(fmt.Errorf("get json schema type: %w", err), fieldType)
		}

		if t, ok := propertySchema["type"].(string); ok {
			switch t {
			case schemaTypeString:
				propertySchema["minLength"] = 1
			case schemaTypeArray:
				propertySchema["minItems"] = 1
			}
		}

		// Apply constraints from the jsonschema tag.
		if jsonschemaTag != nil {
			// explicit format (overrides any inferred format, e.g., time.Time)
			if f := strings.TrimSpace(jsonschemaTag.Format); f != "" {
				propertySchema["format"] = f
			}

			if t, ok := propertySchema["type"].(string); ok {
				switch t {
				case schemaTypeString:
					if minLength := jsonschemaTag.MinLength; minLength != nil {
						propertySchema["minLength"] = *minLength
					}
					if maxLength := jsonschemaTag.MaxLength; maxLength != nil {
						propertySchema["maxLength"] = *maxLength
					}
				case schemaTypeNumber, schemaTypeInteger:
					if minimum := jsonschemaTag.Minimum; minimum != nil {
						propertySchema["minimum"] = *minimum
					}
					if maximum := jsonschemaTag.Maximum; maximum != nil {
						propertySchema["maximum"] = *maximum
					}
				case schemaTypeArray:
					if minItems := jsonschemaTag.MinItems; minItems != nil {
						propertySchema["minItems"] = *minItems
					}
					if maxItems := jsonschemaTag.MaxItems; maxItems != nil {
						propertySchema["maxItems"] = *maxItems
					}
				}
			}
		}

		if isNullable {
			propertySchema = makeNullable(propertySchema)
		}

		properties[identifier] = propertySchema
		if !isOptional {
			requiredProperties = append(requiredProperties, identifier)
		}
	}

	schemaMap["properties"] = properties
	if len(requiredProperties) > 0 {
		schemaMap["required"] = requiredProperties
	} else {
		schemaMap["required"] = []string{}
	}

	schemaMap["additionalProperties"] = additionalProps

	return schemaMap, nil
}

// RenderRoot builds a single JSON Schema document with the provided root type as the top-level schema
// and all discovered interfaces included under $defs. References use local $refs to $defs.
// If root is a slice or array of structs, the top-level schema describes an array whose items
// reference the element type.
func (c *Context) RenderRoot(root reflect.Type) (string, error) {
	root = altshiftReflect.RemoveIndirection(root)

	isArray := false
	elemType := root
	rootKind := root.Kind()

	//exhaustive:ignore
	switch rootKind {
	case reflect.Slice, reflect.Array:
		isArray = true
		elemType = altshiftReflect.RemoveIndirection(root.Elem())
		if elemType.Kind() != reflect.Struct {
			return "", altshiftErrors.NewWithTrace(typeExportErrors.ErrUnsupportedKind, elemType.Kind())
		}
	case reflect.Struct:
		// supported as-is
	default:
		return "", altshiftErrors.NewWithTrace(typeExportErrors.ErrUnsupportedKind, rootKind)
	}

	rootTypeDeclaration, ok := c.TypeDeclarations[elemType]
	if !ok {
		return "", altshiftErrors.NewWithTrace(
			fmt.Errorf("%w (root type)", altshiftErrors.ErrNotInMap),
			elemType,
		)
	}

	rootInterfaceDeclaration, err := utils.ConvertToNonZero[*type_declaration.InterfaceDeclaration](rootTypeDeclaration)
	if err != nil {
		return "", altshiftErrors.New(
			fmt.Errorf("convert to non zero (root type declaration): %w", err),
			rootTypeDeclaration,
		)
	}

	// Build $defs for all interfaces
	defs := map[string]any{}
	for _, typeDeclaration := range c.TypeDeclarationsInOrder {
		interfaceDeclaration, ok := typeDeclaration.(*type_declaration.InterfaceDeclaration)
		if !ok || interfaceDeclaration == nil {
			continue
		}

		schema, err := c.buildInterfaceSchema(interfaceDeclaration)
		if err != nil {
			return "", altshiftErrors.New(fmt.Errorf("build interface schema: %w", err), interfaceDeclaration)
		}

		defs[interfaceDeclaration.Identifier] = schema
	}

	rootInterfaceDeclarationIdentifier := rootInterfaceDeclaration.Identifier

	schemaMap := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$defs":   defs,
	}

	if isArray {
		schemaMap["title"] = rootInterfaceDeclarationIdentifier + "Array"
		schemaMap["type"] = schemaTypeArray
		schemaMap["items"] = map[string]any{"$ref": "#/$defs/" + rootInterfaceDeclarationIdentifier}
	} else {
		schemaMap["title"] = rootInterfaceDeclarationIdentifier
		// Reference the root schema via $defs to avoid duplicating the object at the top level
		schemaMap["$ref"] = "#/$defs/" + rootInterfaceDeclarationIdentifier
	}

	data, err := json.Marshal(schemaMap)
	if err != nil {
		return "", altshiftErrors.NewWithTrace(fmt.Errorf("json marshal (schema map): %w", err), schemaMap)
	}

	return string(data), nil
}
