package type_declaration

import (
	"reflect"

	"github.com/altshiftab/utils_go/pkg/type_export/types/generic_type_info"
)

type PropertySignature struct {
	Identifier string
	Field      *reflect.StructField
	Optional   bool
}

type InterfaceDeclaration struct {
	Identifier string
	// Type is the struct the declaration was made from. The properties are what an exporter
	// usually needs, but what is said about the struct itself -- by a tag on a blank field, which
	// is no property -- is only reachable through the type.
	Type            reflect.Type
	Properties      []*PropertySignature
	GenericTypeInfo *generic_type_info.GenericTypeInfo
}

func (i *InterfaceDeclaration) QualifiedName() string {
	return i.Identifier
}
