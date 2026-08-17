package types

import (
	"fmt"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftUtils "github.com/altshiftab/utils_go/pkg/utils"
)

type Type interface {
	String() (string, error)
}
type BasicType string

func (b BasicType) String() (string, error) { return string(b), nil }

const (
	Boolean         = BasicType("boolean")
	Text            = BasicType("text")
	Real            = BasicType("real")
	DoublePrecision = BasicType("double precision")
	SmallInt        = BasicType("smallint")
	Integer         = BasicType("integer")
	BigInt          = BasicType("bigint")
	Timestamp       = BasicType("timestamptz")
	ByteA           = BasicType("bytea")
	Jsonb           = BasicType("jsonb")
	CiText          = BasicType("citext")
)

type TypeReference struct {
	TypeDeclaration *InterfaceDeclaration
}

func (t *TypeReference) String() (string, error) {
	interfaceDeclaration, err := altshiftUtils.ConvertToNonZero[*InterfaceDeclaration](t.TypeDeclaration)
	if err != nil {
		return "", fmt.Errorf("convert to non zero (type declaration): %w", err)
	}

	idType, err := resolveIdType(interfaceDeclaration)
	if err != nil {
		return "", altshiftErrors.New(fmt.Errorf("resolve id type: %w", err), interfaceDeclaration)
	}
	if idType == "" {
		idType = "uuid"
	}

	return fmt.Sprintf("%s REFERENCES %s(id)", idType, t.TypeDeclaration.QualifiedName()), nil
}

type ArrayType struct {
	ItemsType Type
}

func (a *ArrayType) String() (string, error) {
	typeStr, err := a.ItemsType.String()
	if err != nil {
		return "", fmt.Errorf("items type string: %w", err)
	}

	return fmt.Sprintf("%s[]", typeStr), nil
}
