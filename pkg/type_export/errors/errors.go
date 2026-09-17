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
)
