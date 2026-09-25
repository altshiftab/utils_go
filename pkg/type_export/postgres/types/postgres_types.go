package types

import (
	"fmt"
	"strings"

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

// EnumType is a string limited to a fixed set of values: a text column carrying a CHECK.
type EnumType struct {
	Values []string
}

func (e *EnumType) String() (string, error) { return string(Text), nil }

// quoteLiteral quotes value as a SQL string literal.
func quoteLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// CheckConstraint renders the CHECK limiting column to values: IN for a scalar, containment for an
// array, whose literal is cast to columnType so that a column typed citext[] compares as one.
func CheckConstraint(column string, columnType string, values []string, isArray bool) string {
	literals := make([]string, 0, len(values))
	for _, value := range values {
		literals = append(literals, quoteLiteral(value))
	}
	joined := strings.Join(literals, ", ")

	if isArray {
		return fmt.Sprintf("CHECK (%s <@ ARRAY[%s]::%s)", column, joined, columnType)
	}
	return fmt.Sprintf("CHECK (%s IN (%s))", column, joined)
}

// enumCheck returns the values postgresType limits a column to, and whether the column is an array
// of them; nil when it limits nothing.
func enumCheck(postgresType Type) ([]string, bool) {
	switch typed := postgresType.(type) {
	case *EnumType:
		return typed.Values, false
	case *ArrayType:
		if enumType, ok := typed.ItemsType.(*EnumType); ok {
			return enumType.Values, true
		}
	}
	return nil, false
}
