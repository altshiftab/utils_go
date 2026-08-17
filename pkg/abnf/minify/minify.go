// Package minify rewrites ABNF grammar definitions (RFC 5234, with the
// RFC 7405 char-val extension) into their smallest equivalent form.
//
// Minify strips the whitespace and comments that carry no meaning, which
// leaves the grammar itself untouched. Options.Simplify additionally
// rewrites the expressions of the grammar into shorter equivalent ones,
// which preserves the matched language but changes the structure of the
// paths that parsing produces.
package minify

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/altshiftab/utils_go/pkg/abnf"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// maxSimplificationPasses bounds the passes made to reach a fixed point.
// Simplification is a bottom-up rewrite, so a second pass normally only
// confirms that the first one left nothing behind.
const maxSimplificationPasses = 8

// Options holds the settings of a minification.
type Options struct {
	// Simplify enables rewriting the expressions of the grammar into
	// shorter equivalent ones: dropping redundant parentheses, joining
	// adjacent literals, and expressing numeric values as strings.
	//
	// The matched language is preserved, but the shape of the paths that
	// parsing with the resulting grammar produces is not: dropping a group
	// removes a level from them. Code that walks those paths by structure,
	// rather than by looking up rule names, may need adjusting.
	Simplify bool
	// ExpandLists writes the "#" list operators of RFC 9110 Section 5.6.1
	// out as the standard ABNF they stand for, which makes the definition
	// portable to a parser that only reads RFC 5234. It is off by default
	// because the expansion is far longer than the operator, which is the
	// opposite of what minification is for.
	ExpandLists bool
}

// Minify parses an ABNF grammar definition and returns an equivalent
// definition without the whitespace and comments that carry no meaning.
// Line endings are normalized to the CRLF endings that the specification
// requires, so definitions using LF endings are accepted too.
//
// The result is verified: it is parsed back and re-emitted, and the two
// must agree.
func Minify(input []byte, options *Options) ([]byte, error) {
	grammar, err := abnf.ParseABNF(normalizeLineEndings(input))
	if err != nil {
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf("%w: abnf parse abnf: %w", motmedelErrors.ErrParseError, err),
		)
	}

	output := makeDefinition(grammar, options)

	verificationGrammar, err := abnf.ParseABNF([]byte(output))
	if err != nil {
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf("%w: abnf parse abnf (verification): %w", motmedelErrors.ErrVerificationError, err),
			output,
		)
	}

	if verificationOutput := makeDefinition(verificationGrammar, nil); verificationOutput != output {
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf("%w: the minified grammar does not reproduce itself", motmedelErrors.ErrVerificationError),
			output,
			verificationOutput,
		)
	}

	return []byte(output), nil
}

// makeDefinition returns the minified definition of the grammar, simplifying
// its expressions first if the options ask for it.
func makeDefinition(grammar *abnf.Grammar, options *Options) string {
	if options != nil && options.ExpandLists {
		ExpandLists(grammar)
	}

	if options != nil && options.Simplify {
		transforms := AllTransforms()
		simplifyToFixedPoint(
			func() { simplifyGrammar(grammar, transforms) },
			func() string { return writeGrammar(grammar) },
		)
	}

	return writeGrammar(grammar)
}

// normalizeLineEndings converts the line endings of the input into the CRLF
// endings that RFC 5234 requires, and terminates an unterminated last line.
func normalizeLineEndings(input []byte) []byte {
	normalized := bytes.ReplaceAll(input, []byte("\r\n"), []byte("\n"))
	normalized = bytes.ReplaceAll(normalized, []byte("\n"), []byte("\r\n"))

	if len(normalized) != 0 && !bytes.HasSuffix(normalized, []byte("\r\n")) {
		normalized = append(normalized, '\r', '\n')
	}

	return normalized
}

func writeGrammar(grammar *abnf.Grammar) string {
	var sb strings.Builder
	for _, rule := range grammar.Rules {
		sb.WriteString(writeRule(rule))
	}
	return sb.String()
}

func writeRule(rule *abnf.Rule) string {
	var sb strings.Builder

	sb.WriteString(rule.Name)
	// The whitespace RFC 5234 allows around "=" is not needed: a rulename
	// cannot hold an "=", and no element starts with one.
	sb.WriteByte('=')
	writeAlternation(&sb, rule.Alternation)
	sb.WriteString("\r\n")

	return sb.String()
}

func writeAlternation(sb *strings.Builder, alternation *abnf.Alternation) {
	if alternation == nil {
		return
	}

	for i, concatenation := range alternation.Concatenations {
		if i != 0 {
			// No element starts or ends with a "/", so the whitespace
			// around an alternation separator is not needed either.
			sb.WriteByte('/')
		}
		writeConcatenation(sb, concatenation)
	}
}

func writeConcatenation(sb *strings.Builder, concatenation *abnf.Concatenation) {
	if concatenation == nil {
		return
	}

	for i, repetition := range concatenation.Repetitions {
		if i != 0 {
			// The concatenation separator is the one piece of whitespace
			// that carries meaning: RFC 5234 defines a concatenation as
			// `repetition *(1*c-wsp repetition)`.
			sb.WriteByte(' ')
		}
		writeRepetition(sb, repetition)
	}
}

func writeRepetition(sb *strings.Builder, repetition *abnf.Repetition) {
	if repetition == nil {
		return
	}

	if repetition.Min == repetition.Max {
		if repetition.Min != 1 {
			sb.WriteString(strconv.Itoa(repetition.Min))
		}
	} else {
		if repetition.Min != 0 {
			sb.WriteString(strconv.Itoa(repetition.Min))
		}
		sb.WriteByte('*')
		if repetition.Max != abnf.Inf {
			sb.WriteString(strconv.Itoa(repetition.Max))
		}
	}

	writeElement(sb, repetition.Element)
}

func writeElement(sb *strings.Builder, element abnf.Element) {
	switch v := element.(type) {
	case nil:
		return
	case *abnf.GroupElement:
		sb.WriteByte('(')
		writeAlternation(sb, v.Alternation)
		sb.WriteByte(')')
	case *abnf.OptionElement:
		sb.WriteByte('[')
		writeAlternation(sb, v.Alternation)
		sb.WriteByte(']')
	case *abnf.ListElement:
		// The "#" list operator of RFC 9110 Section 5.6.1, kept as written.
		// Its element is written out here rather than through String, which
		// would space out an alternation within it.
		if v.Min != 0 {
			sb.WriteString(strconv.Itoa(v.Min))
		}
		sb.WriteByte('#')
		if v.Max != abnf.Inf {
			sb.WriteString(strconv.Itoa(v.Max))
		}
		writeElement(sb, v.Element)
	default:
		// A rulename, char-val, num-val or prose-val element holds no
		// whitespace that can be removed.
		sb.WriteString(element.String())
	}
}
