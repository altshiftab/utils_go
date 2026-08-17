package abnf

import (
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

// Path describes a portion of an input that matched a grammar rule, from a
// start index to an end index, with a composite structure.
type Path struct {
	// Subpaths are the ordered child paths.
	Subpaths []*Path
	// MatchRule is the name of the matched rule in the source grammar, or
	// empty for structural paths.
	MatchRule string
	// Start and End delimit the matched input bytes; Start <= End.
	Start, End int
}

// ParseABNF parses an ABNF grammar definition as specified by RFC 5234,
// with the RFC 7405 char-val extension and the RFC 9110 Section 5.6.1 list
// operator. The input must use CRLF line endings, including on the last
// line. The resulting grammar is semantically validated.
func ParseABNF(input []byte) (*Grammar, error) {
	grammar, _, err := ParseABNFWithPath(input)
	if err != nil {
		return nil, err
	}
	return grammar, nil
}

// ParseABNFWithPath parses an ABNF grammar definition as ParseABNF does,
// returning the rulelist path of the definition alongside the grammar. The
// path delimits the input bytes that every construct of the definition
// matched, which callers reporting on a definition need to locate them.
func ParseABNFWithPath(input []byte) (*Grammar, *Path, error) {
	paths, err := Parse(input, abnfGrammar, ruleNameRulelist)
	if err != nil {
		return nil, nil, fmt.Errorf("parse: %w", err)
	}

	var path *Path
	switch len(paths) {
	case 0:
		return nil, nil, ErrNoSolutionFound
	case 1:
		path = paths[0]
	default:
		return nil, nil, &MultipleSolutionsFoundError{Paths: paths}
	}

	grammar, err := evaluateGrammar(input, path)
	if err != nil {
		return nil, nil, fmt.Errorf("evaluate grammar: %w", err)
	}

	if err := validateGrammar(grammar); err != nil {
		return nil, nil, fmt.Errorf("validate grammar: %w", err)
	}

	return grammar, path, nil
}

// Parse parses input using the given grammar, starting from the named root
// rule. It uses a top-down parsing strategy with backtracking and returns
// every solution that consumed the whole input. The grammar must be
// semantically valid, as produced by ParseABNF.
//
// The backtracking makes the worst-case running time exponential in the
// input length; callers parsing untrusted input should bound its length.
// TODO: If hardening against pathological inputs is ever needed, the
// options are, in increasing order of effort:
//  1. An operation budget in the solve functions that turns pathological
//     inputs into an error, without changing matching behavior.
//  2. Memoizing rule results by (rulename, index), which removes the
//     redundant recomputation behind the exponential blowup, but requires
//     making cached paths immutable first (MatchRule is stamped and
//     subpath slices are truncated on shared results after the fact).
//  3. Returning a shared packed parse forest, as the all-solutions return
//     type can itself be exponentially large for ambiguous grammars; this
//     also affects path consumers such as pkg/abnf/utils.
func Parse(input []byte, grammar *Grammar, rootRulename string) ([]*Path, error) {
	if grammar == nil {
		return nil, nil_error.New("grammar")
	}

	rootRule := getRule(rootRulename, grammar.Rulemap)
	if rootRule == nil {
		return nil, &RuleNotFoundError{Rulename: rootRulename}
	}

	var solutions []*Path
	for _, possibility := range solveAlternation(grammar, rootRule.Alternation, input, 0) {
		if possibility.End == len(input) {
			possibility.MatchRule = rootRulename
			solutions = append(solutions, possibility)
		}
	}

	return solutions, nil
}

func solveAlternation(grammar *Grammar, alternation *Alternation, input []byte, index int) []*Path {
	var alternationPaths []*Path

	for _, concatenation := range alternation.Concatenations {
		// Initialize with the first repetition; a concatenation is
		// guaranteed to have at least one.
		var concatenationPaths []*Path
		for _, path := range solveRepetition(grammar, concatenation.Repetitions[0], input, index) {
			concatenationPaths = append(concatenationPaths, &Path{
				Subpaths: []*Path{path},
				Start:    index,
				End:      path.End,
			})
		}

		// Multiply the previous paths with the paths of each following
		// repetition.
		for _, repetition := range concatenation.Repetitions[1:] {
			var nextPaths []*Path
			for _, concatenationPath := range concatenationPaths {
				for _, path := range solveRepetition(grammar, repetition, input, concatenationPath.End) {
					// An empty path does not extend the previous path.
					if path.Start == path.End {
						nextPaths = append(nextPaths, concatenationPath)
						continue
					}

					// Drop a trailing empty-traversal subpath before
					// extending.
					subpaths := slices.Clone(concatenationPath.Subpaths)
					if len(subpaths) != 0 {
						if lastSubpath := subpaths[len(subpaths)-1]; lastSubpath.Start == lastSubpath.End {
							subpaths = subpaths[:len(subpaths)-1]
						}
					}

					nextPaths = append(nextPaths, &Path{
						Subpaths: append(subpaths, path),
						Start:    index,
						End:      path.End,
					})
				}
			}
			concatenationPaths = nextPaths
		}

		alternationPaths = append(alternationPaths, concatenationPaths...)
	}

	return alternationPaths
}

func solveRepetition(grammar *Grammar, repetition *Repetition, input []byte, index int) []*Path {
	var paths []*Path

	// If the empty solution is possible, keep track of it.
	if repetition.Min == 0 {
		paths = append(paths, &Path{
			Subpaths: []*Path{{Start: index, End: index}},
			Start:    index,
			End:      index,
		})
	}

	if !shouldContinueRepetition(repetition, input, index, 0) {
		return paths
	}
	iterationPaths := [][]*Path{solveElement(grammar, repetition.Element, input, index)}

	for i := 1; i != repetition.Max; i++ {
		var currentPaths []*Path
		for _, previousPath := range iterationPaths[i-1] {
			if !shouldContinueRepetition(repetition, input, previousPath.End, i) {
				continue
			}
			for _, elementPath := range solveElement(grammar, repetition.Element, input, previousPath.End) {
				subpaths := make([]*Path, 0, len(previousPath.Subpaths)+1)
				subpaths = append(subpaths, previousPath.Subpaths...)
				subpaths = append(subpaths, elementPath)

				currentPaths = append(currentPaths, &Path{
					Subpaths: subpaths,
					Start:    previousPath.Start,
					End:      elementPath.End,
				})
			}
		}

		// If no new path was found during this iteration, stop.
		if len(currentPaths) == 0 {
			break
		}
		iterationPaths = append(iterationPaths, currentPaths)
	}

	// Collect the iteration counts that satisfy the repetition bounds.
	for i := max(1, repetition.Min); i <= len(iterationPaths); i++ {
		paths = append(paths, iterationPaths[i-1]...)
	}

	return paths
}

func solveElement(grammar *Grammar, element Element, input []byte, index int) []*Path {
	switch v := element.(type) {
	case *RulenameElement:
		rule := getRule(v.Name, grammar.Rulemap)
		if rule == nil {
			return nil
		}
		var paths []*Path
		for _, path := range solveAlternation(grammar, rule.Alternation, input, index) {
			path.MatchRule = v.Name
			paths = append(paths, path)
		}
		return paths

	case *OptionElement:
		return solveRepetition(
			grammar,
			&Repetition{Min: 0, Max: 1, Element: &GroupElement{Alternation: v.Alternation}},
			input,
			index,
		)

	case *GroupElement:
		return solveAlternation(grammar, v.Alternation, input, index)

	case *ListElement:
		return solveAlternation(grammar, v.alternation(), input, index)

	case *CharValElement:
		end := index + len(v.Value)
		if end > len(input) {
			return nil
		}
		for i := range len(v.Value) {
			inputByte, valueByte := input[index+i], v.Value[i]
			if !v.Sensitive {
				inputByte, valueByte = asciiLower(inputByte), asciiLower(valueByte)
			}
			if inputByte != valueByte {
				return nil
			}
		}
		return []*Path{{Start: index, End: end}}

	case *NumValElement:
		switch v.Status {
		case NumValStatusRange:
			low := mustParseNumVal(v.Values[0], v.Base)
			high := mustParseNumVal(v.Values[1], v.Base)

			r, size := utf8.DecodeRune(input[index:]) //nolint:nilaway // A nil input []byte is valid; slicing it yields an empty slice, handled below.
			if r == utf8.RuneError && size <= 1 {
				// Empty or invalid UTF-8 input.
				return nil
			}
			if low <= r && r <= high {
				return []*Path{{Start: index, End: index + size}}
			}

		case NumValStatusSeries:
			// Only match if all values match in order.
			end := index
			for _, value := range v.Values {
				expected := string(mustParseNumVal(value, v.Base))
				if end+len(expected) > len(input) || string(input[end:end+len(expected)]) != expected {
					return nil
				}
				end += len(expected)
			}
			return []*Path{{Start: index, End: end}}
		}

	case *ProseValElement:
		// A prose-val is a prose description and cannot be matched.
	}

	return nil
}

// shouldContinueRepetition returns whether another iteration of the
// repetition should be attempted at the given input index.
func shouldContinueRepetition(repetition *Repetition, input []byte, index, iteration int) bool {
	couldMatch := true
	switch v := repetition.Element.(type) {
	case *NumValElement:
		couldMatch = index < len(input)
	case *CharValElement:
		couldMatch = index+len(v.Value) <= len(input)
	case *ProseValElement:
		couldMatch = false
	}

	// An unbounded repetition is only limited by the input length.
	if repetition.Max == Inf {
		return couldMatch
	}
	return couldMatch && iteration < repetition.Max
}

func asciiLower(b byte) byte {
	if 'A' <= b && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}
