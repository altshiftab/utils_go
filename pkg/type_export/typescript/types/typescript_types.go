package types

import (
	"fmt"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	typescriptErrors "github.com/altshiftab/utils_go/pkg/type_export/typescript/errors"
)

type Type interface {
	String() (string, error)
}

type TypeDeclaration interface {
	TypeReference() *TypeReference
	QualifiedName() string
}

type TypeReference struct {
	TypeDeclaration TypeDeclaration
	TypeArguments   []Type
}

func (t *TypeReference) String() (string, error) {
	name := t.TypeDeclaration.QualifiedName()
	if len(t.TypeArguments) == 0 {
		return name, nil
	}

	args := make([]string, 0, len(t.TypeArguments))
	for _, a := range t.TypeArguments {
		typeStr, err := a.String()
		if err != nil {
			return "", fmt.Errorf("type string: %w", err)
		}
		args = append(args, typeStr)
	}

	return fmt.Sprintf("%s<%s>", name, strings.Join(args, ", ")), nil
}

type TypeParameter struct {
	Identifier string
}

func (p *TypeParameter) String() (string, error) { return p.Identifier, nil }

type BasicType string

const (
	Boolean    = BasicType("boolean")
	Number     = BasicType("number")
	String     = BasicType("string")
	Null       = BasicType("null")
	Any        = BasicType("any")
	Uint8Array = BasicType("Uint8Array")
)

func (b BasicType) String() (string, error) { return string(b), nil }

type UnionType struct {
	Types []Type
}

func (u UnionType) String() (string, error) {
	var tsTypes []string
	for _, t := range u.Types {
		typeStr, err := t.String()
		if err != nil {
			return "", fmt.Errorf("type string: %w", err)
		}
		tsTypes = append(tsTypes, typeStr)
	}
	return strings.Join(tsTypes, " | "), nil
}

type MapType struct {
	IndexType Type
	ValueType Type
}

func (m *MapType) String() (string, error) {
	indexTypeString, err := m.IndexType.String()
	if err != nil {
		return "", fmt.Errorf("index type string: %w", err)
	}

	if indexTypeString != "number" && indexTypeString != "string" {
		return "", altshiftErrors.NewWithTrace(typescriptErrors.ErrUnsupportedIndexType, indexTypeString)
	}

	valueTypeString, err := m.ValueType.String()
	if err != nil {
		return "", fmt.Errorf("value type string: %w", err)
	}

	return fmt.Sprintf("{ [key: %s]: %s }", indexTypeString, valueTypeString), nil
}

type ArrayType struct {
	ItemsType Type
}

func (a *ArrayType) String() (string, error) {
	fmtStr := "%s[]"
	if _, ok := a.ItemsType.(*UnionType); ok {
		fmtStr = "(%s)[]"
	}

	typeStr, err := a.ItemsType.String()
	if err != nil {
		return "", fmt.Errorf("items type string: %w", err)
	}

	return fmt.Sprintf(fmtStr, typeStr), nil
}
