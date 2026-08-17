package generic_type_info

import "github.com/altshiftab/utils_go/pkg/type_export/types/shape"

type GenericTypeInfo struct {
	TypeParameterNames           []string
	FieldNameToShapes            map[string][]shape.Shape
	TypeParameterNameToFieldName map[string]string
}
