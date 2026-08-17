package lint

import (
	"errors"
	"fmt"

	"github.com/altshiftab/utils_go/pkg/abnf"
)

// holdsListOperator reports whether a definition uses the "#" list operator
// of RFC 9110 Section 5.6.1 anywhere.
//
// The scan skips comments, char-vals and prose-vals, where a "#" is just a
// character: the token character sets of the HTTP grammars are full of them.
func holdsListOperator(input []byte) bool {
	for i := 0; i < len(input); i++ {
		switch input[i] {
		case ';':
			// A comment runs to the end of its line.
			for i < len(input) && input[i] != '\n' {
				i++
			}
		case '"':
			i++
			for i < len(input) && input[i] != '"' {
				i++
			}
		case '<':
			for i < len(input) && input[i] != '>' {
				i++
			}
		case '#':
			return true
		}
	}

	return false
}

// holdsListRepeat reports whether a rule uses the "#" list operator, which
// pkg/abnf reads as the standard ABNF that RFC 7230 Section 7 expands it to.
func holdsListRepeat(path *abnf.Path, input []byte) bool {
	for _, repeat := range items(path, nodeRepeat) {
		for i := repeat.Start; i < repeat.End && i < len(input); i++ {
			if input[i] == '#' {
				return true
			}
		}
	}

	return false
}

// explainListSeparator adds what a reader needs to a parse failure that the
// list expansion caused. The expansion refers to OWS, which a grammar using
// the operator has to define, just as it defines every other rule it refers
// to; a reader who never typed "OWS" needs telling where it came from.
func explainListSeparator(input []byte, err error) error {
	if !holdsListOperator(input) {
		return err
	}

	notFound, ok := errors.AsType[*abnf.DependencyNotFoundError](err)
	if !ok || !equalFoldAscii(notFound.Rulename, listSeparatorRulename) {
		return err
	}

	return fmt.Errorf(
		"%w (the \"#\" list operator expands to ABNF separating elements with %s, "+
			"which RFC 9110 Section 5.6.3 defines as `%s = *( SP / HTAB )`)",
		err,
		listSeparatorRulename,
		listSeparatorRulename,
	)
}

// listSeparatorRulename is the rule the list expansion separates elements
// with.
const listSeparatorRulename = "OWS"

func equalFoldAscii(a string, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range len(a) {
		if lowerAscii(a[i]) != lowerAscii(b[i]) {
			return false
		}
	}
	return true
}

func lowerAscii(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}
