// Package enum lets a string type declare the values it may hold, for every type_export producer
// to render: an enum in JSON schema, a union in TypeScript and a CHECK in Postgres. TypeScript also
// declares each value as a constant, under a name derived from it or one the type gives.
package enum

import (
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"

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

// Named is implemented by an Enum that names its values itself, rather than taking the names
// Members derives from them. EnumNames maps each value to its name, and is called on a zero value,
// as EnumValues is.
type Named interface {
	Enum
	EnumNames() map[string]string
}

var namedType = reflect.TypeFor[Named]()

// identifierPattern is what a name must match: an identifier in Go and in TypeScript alike.
var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// snakeCasePattern is what a value must match for Members to derive a name from it.
var snakeCasePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// Member is a value an enum declares and the name it goes by.
type Member struct {
	Name  string
	Value string
}

// Members returns the values reflectType declares, after removing pointer indirection, each paired
// with a name, in the order of the values: what TypeScript declares them as constants under, so
// that a value written role.Admin in Go is Role.Admin there too.
//
// A type implementing Named gives the names itself, and then every value must have one, every name
// a value, and no two values the same name. Otherwise each name is derived from its value, so that
// "dd_admin" goes by DdAdmin, as a Go constant named after its value would; when a value is not
// snake_case, or two values would go by the same name, Members returns nil, as it does for a type
// declaring no values.
func Members(reflectType reflect.Type) ([]*Member, error) {
	values, err := Values(reflectType)
	if err != nil {
		return nil, fmt.Errorf("values: %w", err)
	}
	if values == nil {
		return nil, nil
	}

	for reflectType.Kind() == reflect.Pointer {
		reflectType = reflectType.Elem()
	}

	if !reflect.PointerTo(reflectType).Implements(namedType) {
		return derivedMembers(values), nil
	}

	namedValue, ok := reflect.New(reflectType).Interface().(Named)
	if !ok {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: named type assertion", altshiftErrors.ErrConversionNotOk), reflectType.String(),
		)
	}

	names := namedValue.EnumNames()

	members := make([]*Member, 0, len(values))
	valueByName := make(map[string]string, len(values))
	for _, value := range values {
		name, ok := names[value]
		if !ok {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %q", typeExportErrors.ErrEnumUnnamedValue, value), reflectType.String(),
			)
		}
		if !identifierPattern.MatchString(name) {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %q", typeExportErrors.ErrEnumInvalidName, name), reflectType.String(),
			)
		}
		if otherValue, ok := valueByName[name]; ok {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %q for %q and %q", typeExportErrors.ErrEnumDuplicateName, name, otherValue, value),
				reflectType.String(),
			)
		}
		valueByName[name] = value

		members = append(members, &Member{Name: name, Value: value})
	}

	for _, value := range slices.Sorted(maps.Keys(names)) {
		if !slices.Contains(values, value) {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %q", typeExportErrors.ErrEnumNamedNonValue, value), reflectType.String(),
			)
		}
	}

	return members, nil
}

// derivedMembers names each value by its snake_case words, capitalised and joined, and returns nil
// when a value is not snake_case or two values come out alike.
func derivedMembers(values []string) []*Member {
	members := make([]*Member, 0, len(values))
	derivedNames := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !snakeCasePattern.MatchString(value) {
			return nil
		}

		var builder strings.Builder
		for word := range strings.SplitSeq(value, "_") {
			builder.WriteString(strings.ToUpper(word[:1]))
			builder.WriteString(word[1:])
		}
		name := builder.String()

		if _, ok := derivedNames[name]; ok {
			return nil
		}
		derivedNames[name] = struct{}{}

		members = append(members, &Member{Name: name, Value: value})
	}

	return members
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
