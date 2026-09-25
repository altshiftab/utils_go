// Package enum lets a string type declare the values it may hold, for every type_export producer
// to render: an enum in JSON schema, a union in TypeScript and a CHECK in Postgres.
package enum

import (
	"fmt"
	"reflect"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	altshiftReflect "github.com/altshiftab/utils_go/pkg/reflect"
	typeExportErrors "github.com/altshiftab/utils_go/pkg/type_export/errors"
)

// Enum is implemented by a string type whose values are limited to a fixed set. EnumValues is
// called on a zero value, so it must not depend on its receiver.
type Enum interface {
	EnumValues() []string
}

var enumType = reflect.TypeFor[Enum]()

// Values returns the values reflectType declares through Enum, after removing pointer
// indirection, and nil when it declares none.
func Values(reflectType reflect.Type) ([]string, error) {
	if reflectType == nil {
		return nil, nil
	}

	for reflectType.Kind() == reflect.Pointer {
		reflectType = reflectType.Elem()
	}

	if !reflect.PointerTo(reflectType).Implements(enumType) {
		return nil, nil
	}

	if reflectType.Kind() != reflect.String {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %s", typeExportErrors.ErrEnumNotString, reflectType),
		)
	}

	enumValue, ok := reflect.New(reflectType).Interface().(Enum)
	if !ok {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: enum type assertion", altshiftErrors.ErrConversionNotOk), reflectType.String(),
		)
	}

	values := enumValue.EnumValues()
	if len(values) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("enum values"), reflectType.String())
	}

	return values, nil
}

// TagIsSlice checks that an enum tag may constrain a field of fieldType, and reports whether it
// constrains the items of a slice rather than the field itself. Only plain string qualifies, as the
// field or as its slice's element, pointers removed; a named type declares its values through Enum.
func TagIsSlice(fieldType reflect.Type) (bool, error) {
	if fieldType == nil {
		return false, altshiftErrors.NewWithTrace(fmt.Errorf("%w: nil field type", typeExportErrors.ErrUnsupportedKind))
	}

	elementType := altshiftReflect.RemoveIndirection(fieldType)
	isSlice := false
	if kind := elementType.Kind(); kind == reflect.Slice || kind == reflect.Array {
		isSlice = true
		elementType = altshiftReflect.RemoveIndirection(elementType.Elem())
	}

	if elementType.Kind() != reflect.String {
		return false, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: enum tag on %s", typeExportErrors.ErrUnsupportedKind, elementType),
		)
	}
	if elementType != reflect.TypeFor[string]() {
		return false, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %s", typeExportErrors.ErrEnumTagOnNamedType, elementType),
		)
	}

	return isSlice, nil
}
