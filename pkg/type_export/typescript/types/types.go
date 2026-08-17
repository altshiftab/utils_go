package types

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	typeExportErrors "github.com/altshiftab/utils_go/pkg/type_export/errors"
	typeExportContext "github.com/altshiftab/utils_go/pkg/type_export/types/context"
	"github.com/altshiftab/utils_go/pkg/type_export/types/shape"
	"github.com/altshiftab/utils_go/pkg/type_export/types/type_declaration"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	motmedelJsonTag "github.com/altshiftab/utils_go/pkg/json/types/tag"
	motmedelReflect "github.com/altshiftab/utils_go/pkg/reflect"
	jsonschemaTag "github.com/altshiftab/utils_go/pkg/type_export/jsonschema/types/tag"
	"github.com/altshiftab/utils_go/pkg/utils"
)

func isTime(t reflect.Type) bool {
	return t.Name() == "Time" && t.PkgPath() == "time"
}

func isNumber(kind reflect.Kind) bool {
	return numberKinds[kind]
}

// nonNumberPrimitiveKinds is the non-number set of Kinds that we support converting to TypeScript.
var nonNumberPrimitiveKinds = map[reflect.Kind]bool{
	reflect.Bool:    true,
	reflect.Uintptr: true,
	reflect.String:  true,
}

var numberKinds = map[reflect.Kind]bool{
	reflect.Int:     true,
	reflect.Int8:    true,
	reflect.Int16:   true,
	reflect.Int32:   true,
	reflect.Int64:   true,
	reflect.Uint:    true,
	reflect.Uint8:   true,
	reflect.Uint16:  true,
	reflect.Uint32:  true,
	reflect.Uint64:  true,
	reflect.Float32: true,
	reflect.Float64: true,
}

var validTSIdentifier = regexp.MustCompile(`^[a-zA-Z_$][a-zA-Z0-9_$]*$`)

func quotePropertyName(name string) string {
	if validTSIdentifier.MatchString(name) {
		return name
	}
	return fmt.Sprintf(`"%s"`, name)
}

func isPrimitive(kind reflect.Kind) bool {
	return numberKinds[kind] || nonNumberPrimitiveKinds[kind]
}

func isPrimitiveAlias(reflectType reflect.Type) bool {
	return isPrimitive(reflectType.Kind()) && reflectType.Name() != reflectType.Kind().String()
}

type Context struct {
	*typeExportContext.Context
	GenerateNominalTypes bool
}

func (c *Context) GetTypeScriptType(reflectType reflect.Type) (Type, error) {
	typeScriptType, err := c.getUnderlyingTypeScriptType(reflectType)
	if err != nil {
		return nil, err
	}

	reflectType = motmedelReflect.RemoveIndirection(reflectType)

	// Struct types are already returned as a TypeReference by
	// getUnderlyingTypeScriptType (their TypeDeclaration is an
	// *InterfaceDeclaration, not a *TypeAliasDeclaration), so the lookup below
	// would fail. The alias path is only for named non-struct types like
	// `type Score int` or `type Names []string`.
	useTypeAlias := reflectType.Name() != "" &&
		reflectType.Kind() != reflect.Struct &&
		(!isPrimitive(reflectType.Kind()) || isPrimitiveAlias(reflectType)) &&
		!isTime(reflectType)

	if useTypeAlias {
		typeDeclaration, err := utils.MapGet(c.TypeDeclarations, reflectType)
		if err != nil {
			return nil, motmedelErrors.New(fmt.Errorf("map get: %w", err), c.TypeDeclarations, reflectType)
		}

		typeAliasDeclaration, err := utils.Convert[*type_declaration.TypeAliasDeclaration](typeDeclaration)
		if err != nil {
			return nil, motmedelErrors.New(fmt.Errorf("convert: %w", err), typeDeclaration)
		}

		return (&TypeAliasDeclaration{TypeAliasDeclaration: typeAliasDeclaration, c: c}).TypeReference(), nil
	}

	return typeScriptType, nil
}

// getUnderlyingTypeScriptType returns the structural TypeScript type for a Go
// reflect.Type without wrapping it in a TypeReference even when the type is a
// named alias. Used when emitting the right-hand side of an `export type X = …`
// declaration, where wrapping would produce `export type X = X;`.
func (c *Context) getUnderlyingTypeScriptType(reflectType reflect.Type) (Type, error) {
	reflectType = motmedelReflect.RemoveIndirection(reflectType)

	var typeScriptType Type
	//exhaustive:ignore
	switch kind := reflectType.Kind(); kind {
	case reflect.Struct:
		if isTime(reflectType) {
			typeScriptType = String
		} else {
			typeDeclaration, err := utils.MapGetNonZero(c.TypeDeclarations, reflectType)
			if err != nil {
				return nil, motmedelErrors.New(fmt.Errorf("map get non zero: %w", err), c.TypeDeclarations, reflectType)
			}

			interfaceDeclaration, err := utils.Convert[*type_declaration.InterfaceDeclaration](typeDeclaration)
			if err != nil {
				return nil, motmedelErrors.New(fmt.Errorf("convert: %w", err), typeDeclaration)
			}

			typeReference := (&InterfaceDeclaration{InterfaceDeclaration: interfaceDeclaration, c: c}).TypeReference()

			if genericTypeInfo := interfaceDeclaration.GenericTypeInfo; genericTypeInfo != nil {
				var typeArguments []Type
				for _, typeParameterName := range genericTypeInfo.TypeParameterNames {
					// Find a field in the generic struct that uses the generic type parameter.

					typeParameterNameToFieldName := genericTypeInfo.TypeParameterNameToFieldName
					fieldName, err := utils.MapGet(typeParameterNameToFieldName, typeParameterName)
					if err != nil {
						return nil, motmedelErrors.New(
							fmt.Errorf("map get: %w", err),
							typeParameterNameToFieldName, typeParameterName,
						)
					}

					field, ok := reflectType.FieldByName(fieldName)
					if !ok {
						return nil, motmedelErrors.NewWithTrace(
							typeExportErrors.ErrNoStructField,
							reflectType, fieldName,
						)
					}

					// Determine the "shape" of the field that uses the generic type parameter and extract the concrete type
					// for this instantiation.

					argReflectType := field.Type
					for _, fieldShape := range genericTypeInfo.FieldNameToShapes[fieldName] {
						if fieldShape.Param != typeParameterName {
							continue
						}
						switch fieldShape.Kind {
						case shape.KindPointer:
							argReflectType = motmedelReflect.RemoveIndirection(argReflectType)
						case shape.KindSlice, shape.KindArray:
							argReflectType = argReflectType.Elem()
						case shape.KindMapValue:
							argReflectType = argReflectType.Elem()
						case shape.KindMapKey:
							argReflectType = argReflectType.Key()
						case shape.KindDirect:
							// use as-is
						}
						break
					}

					typeArgument, err := c.GetTypeScriptType(argReflectType)
					if err != nil {
						return nil, fmt.Errorf("get type script type: %w", err)
					}
					typeArguments = append(typeArguments, typeArgument)
				}

				// Reference the generic interface with the concrete types.
				typeReference.TypeArguments = typeArguments
			}

			return typeReference, nil
		}

	case reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uint,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Int,
		reflect.Float32,
		reflect.Float64:
		typeScriptType = Number
	case reflect.String:
		typeScriptType = String
	case reflect.Bool:
		typeScriptType = Boolean
	case reflect.Map:
		// TypeScript index signature parameter types[1] must be either "string" or "number", and
		// cannot be type aliases, otherwise the TypeScript compiler will fail with error
		// "TS1336: An index signature parameter type cannot be a type alias.".
		//
		// Example:
		//
		//   export type Foo = string;
		//   export type Bar = { [key: Foo]: string };  // Compiler produces error TS1336.
		//
		// Thus, we treat map keys as a special case where we ignore any type aliases and use either
		// "string" or "number" directly.
		//
		// [1] https://www.typescriptlang.org/docs/handbook/advanced-types.html#index-types-and-index-signatures.
		var indexType Type
		reflectTypeKeyKind := reflectType.Key().Kind()
		if reflectTypeKeyKind == reflect.String {
			indexType = String
		} else if isNumber(reflectTypeKeyKind) {
			indexType = Number
		} else {
			return nil, motmedelErrors.NewWithTrace(
				fmt.Errorf("%w: %T", typeExportErrors.ErrUnsupportedKind, reflectTypeKeyKind),
				reflectTypeKeyKind,
			)
		}

		valueType, err := c.GetTypeScriptType(reflectType.Elem())
		if err != nil {
			return nil, err
		}

		typeScriptType = &MapType{IndexType: indexType, ValueType: valueType}
	case reflect.Slice, reflect.Array:
		// []byte carries binary data, not an array of numbers.
		if reflectType.Elem().Kind() == reflect.Uint8 {
			typeScriptType = Uint8Array
		} else {
			itemsType, err := c.GetTypeScriptType(reflectType.Elem())
			if err != nil {
				return nil, err
			}
			typeScriptType = &ArrayType{ItemsType: itemsType}
		}

	case reflect.Interface:
		typeScriptType = Any
	default:
		return nil, motmedelErrors.NewWithTrace(fmt.Errorf("%w: %T", typeExportErrors.ErrUnsupportedKind, kind), kind)
	}

	return typeScriptType, nil
}

func (c *Context) Render() (string, error) {
	var interfaceDeclarations []*InterfaceDeclaration
	var typeAliasDeclarations []*TypeAliasDeclaration

	for _, typeDeclaration := range c.TypeDeclarationsInOrder {
		switch v := any(typeDeclaration).(type) {
		case *type_declaration.InterfaceDeclaration:
			interfaceDeclarations = append(
				interfaceDeclarations,
				&InterfaceDeclaration{InterfaceDeclaration: v, c: c},
			)
		case *type_declaration.TypeAliasDeclaration:
			typeAliasDeclarations = append(
				typeAliasDeclarations,
				&TypeAliasDeclaration{TypeAliasDeclaration: v, c: c},
			)
		}
	}

	var stringBuilder strings.Builder

	for i, interfaceDeclaration := range interfaceDeclarations {
		if i > 0 {
			stringBuilder.WriteString("\n")
		}
		d, err := interfaceDeclaration.String()
		if err != nil {
			return "", motmedelErrors.New(fmt.Errorf("to type script: %w", err), interfaceDeclaration)
		}
		stringBuilder.WriteString(d)
		stringBuilder.WriteString("\n")
	}

	for _, typeAliasDeclaration := range typeAliasDeclarations {
		if len(interfaceDeclarations) > 0 {
			stringBuilder.WriteString("\n")
		}

		d, err := typeAliasDeclaration.ToTypeScript()
		if err != nil {
			return "", motmedelErrors.New(fmt.Errorf("to type script: %w", err), typeAliasDeclaration)
		}
		stringBuilder.WriteString(d)
		stringBuilder.WriteString("\n")
	}

	return stringBuilder.String(), nil
}

func renderTypeParams(params []string) string {
	if len(params) == 0 {
		return ""
	}
	return fmt.Sprintf("<%s>", strings.Join(params, ", "))
}

type InterfaceDeclaration struct {
	*type_declaration.InterfaceDeclaration
	c *Context
}

func (t *InterfaceDeclaration) String() (string, error) {
	var propertyStrings []string

	var typeParameters []string
	genericTypeInfo := t.GenericTypeInfo
	if genericTypeInfo != nil {
		typeParameters = genericTypeInfo.TypeParameterNames
	}

	for _, property := range t.Properties {
		if property == nil {
			continue
		}

		field := property.Field
		if field == nil {
			return "", motmedelErrors.NewWithTrace(nil_error.New("property field"), property)
		}

		identifier := property.Identifier
		optional := property.Optional

		fieldTag := field.Tag

		rawSchemaTag := fieldTag.Get("jsonschema")
		schemaTag, err := jsonschemaTag.New(rawSchemaTag)
		if err != nil {
			return "", motmedelErrors.New(fmt.Errorf("jsonschema tag new: %w", err), schemaTag)
		}
		if schemaTag != nil {
			if schemaTag.Skip {
				continue
			}

			if name := schemaTag.Name; name != "" {
				identifier = name
			}

			optional = optional || schemaTag.Optional
		} else {
			jsonTag := motmedelJsonTag.New(fieldTag.Get("json"))
			if jsonTag != nil {
				if jsonTag.Skip {
					continue
				}
				if name := jsonTag.Name; name != "" {
					identifier = name
				}

				optional = optional || jsonTag.OmitEmpty || jsonTag.OmitZero
			}
		}

		optionalString := ""
		if optional {
			optionalString = "?"
		}

		typeScriptType, err := t.c.GetTypeScriptType(field.Type)
		if err != nil {
			return "", fmt.Errorf("get type script type: %w", err)
		}

		// Replace the field's type with the generic type parameter if the field uses the generic type parameter.

		if genericTypeInfo != nil {
			for _, fieldShape := range genericTypeInfo.FieldNameToShapes[property.Identifier] {
				switch fieldShape.Kind {
				case shape.KindDirect, shape.KindPointer:
					typeScriptType = &TypeParameter{Identifier: fieldShape.Param}
				case shape.KindSlice, shape.KindArray:
					typeScriptType = &ArrayType{ItemsType: &TypeParameter{Identifier: fieldShape.Param}}
				case shape.KindMapValue:
					// Preserve the discovered key type from the reflect-populated MapType, replace value with param.
					mapType, err := utils.ConvertToNonZero[*MapType](typeScriptType)
					if err != nil {
						return "", fmt.Errorf("convert to non zero: %w", err)
					}
					mapType.ValueType = &TypeParameter{Identifier: fieldShape.Param}
				case shape.KindMapKey:
					// If the key is parameterized, ensure index is the parameter and keep value as-is.
					mapType, err := utils.ConvertToNonZero[*MapType](typeScriptType)
					if err != nil {
						return "", fmt.Errorf("convert to non zero: %w", err)
					}
					mapType.IndexType = &TypeParameter{Identifier: fieldShape.Param}
				}
			}
		}

		typeString, err := typeScriptType.String()
		if err != nil {
			return "", fmt.Errorf("type string: %w", err)
		}

		propertyStrings = append(
			propertyStrings,
			fmt.Sprintf("\t%s%s: %s;\n", quotePropertyName(identifier), optionalString, typeString),
		)
	}

	return fmt.Sprintf(
		"export interface %s%s {\n%s}",
		t.Identifier,
		renderTypeParams(typeParameters),
		strings.Join(propertyStrings, ""),
	), nil
}

func (t *InterfaceDeclaration) QualifiedName() string {
	return t.Identifier
}

func (t *InterfaceDeclaration) TypeReference() *TypeReference {
	return &TypeReference{TypeDeclaration: t}
}

type TypeAliasDeclaration struct {
	*type_declaration.TypeAliasDeclaration
	c *Context
}

func (a *TypeAliasDeclaration) TypeReference() *TypeReference {
	return &TypeReference{TypeDeclaration: a}
}

func (a *TypeAliasDeclaration) QualifiedName() string {
	return a.Identifier
}

func (a *TypeAliasDeclaration) ToTypeScript() (string, error) {
	params := renderTypeParams(a.TypeParameters)

	typeScriptType, err := a.c.getUnderlyingTypeScriptType(a.ReflectType)
	if err != nil {
		return "", fmt.Errorf("get underlying type script type: %w", err)
	}

	param, err := typeScriptType.String()
	if err != nil {
		return "", fmt.Errorf("type script type string: %w", err)
	}

	if _, ok := typeScriptType.(*UnionType); !ok && a.c.GenerateNominalTypes {
		return fmt.Sprintf(`    export type %s%s = %s & {
		/**
		* WARNING: Do not reference this field from application code.
		*
		* This field exists solely to provide nominal typing. For reference, see
		* https://www.typescriptlang.org/play#example/nominal-typing.
		*/
        _%sbrand: 'type alias for %s'
    };

    export function %s%s(v: %s): %s%s {
        return v as %s%s;
    };
`,
			a.Identifier,
			params,
			param,
			strings.ToLower(string(a.Identifier[0]))+a.Identifier[1:],
			param,
			a.Identifier,
			params,
			param,
			a.Identifier,
			params,
			a.Identifier,
			params), nil
	}

	return fmt.Sprintf("export type %s%s = %s;", a.Identifier, params, param), nil
}
