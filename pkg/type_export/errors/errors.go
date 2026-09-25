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
	// ErrUnknownTagOption is for a tag option no producer reads, which would otherwise be
	// dropped without a word.
	ErrUnknownTagOption = errors.New("unknown tag option")
)
