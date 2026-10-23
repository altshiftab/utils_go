package errors

import (
	"errors"
)

var (
	ErrNoStructField   = errors.New("no struct field")
	ErrUnsupportedKind = errors.New("unsupported kind")
	// ErrRefPrefixWithRoot is for rendering a whole document from a context whose references were
	// pointed somewhere else. The document keeps its schemas under $defs, so references written
	// against another prefix would resolve to nothing within it.
	ErrRefPrefixWithRoot = errors.New("ref prefix set while rendering a root document")
	// ErrEnumNotString is for a type declaring enum values whose kind is not string.
	ErrEnumNotString = errors.New("enum type is not a string kind")
	// ErrEnumTagOnNamedType is for an enum tag on a field of a named type. A named type declares
	// its own values, and a tag narrowing them would be a second source for one set.
	ErrEnumTagOnNamedType = errors.New("enum tag on a field of a named type")
	// ErrEnumUnnamedValue is for an enum that names its values but leaves one of them unnamed.
	ErrEnumUnnamedValue = errors.New("enum value has no name")
	// ErrEnumNamedNonValue is for an enum naming a value it does not declare, a name no producer
	// would render.
	ErrEnumNamedNonValue = errors.New("enum name for an undeclared value")
	// ErrEnumDuplicateName is for an enum giving two of its values the same name.
	ErrEnumDuplicateName = errors.New("enum name given to more than one value")
	// ErrEnumInvalidName is for an enum name that is not an identifier, and so cannot be referred to
	// as one.
	ErrEnumInvalidName = errors.New("enum name is not an identifier")
	// ErrUnknownTagOption is for a tag option no producer reads, which would otherwise be
	// dropped without a word.
	ErrUnknownTagOption = errors.New("unknown tag option")
)
