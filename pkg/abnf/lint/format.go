package lint

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/altshiftab/utils_go/pkg/abnf"
	"github.com/altshiftab/utils_go/pkg/abnf/minify"
)

// Rule names of the ABNF grammar of ABNF, as they appear in the MatchRule of
// a parsed path.
const (
	nodeRule          = "rule"
	nodeRulename      = "rulename"
	nodeDefinedAs     = "defined-as"
	nodeElements      = "elements"
	nodeAlternation   = "alternation"
	nodeConcatenation = "concatenation"
	nodeRepetition    = "repetition"
	nodeRepeat        = "repeat"
	nodeGroup         = "group"
	nodeOption        = "option"
	nodeCharVal       = "char-val"
	nodeNumVal        = "num-val"
	nodeProseVal      = "prose-val"
)

// crlf is what minification puts at the end of every line.
const crlf = "\r\n"

// piece is one place where a definition says more than minification needs
// it to. Pieces are found precisely and then gathered into one finding per
// rule, which is the unit minification rewrites: reporting every single
// space on its own buries a reader in a definition that is merely spaced
// out.
type piece struct {
	ruleId RuleId
	// start and end delimit the normalized bytes the piece covers.
	start, end  int
	replacement string
}

// pieceRuleIds orders the checks a piece can answer to by how much they say
// about the rule holding them; the first one present names the finding.
var pieceRuleIds = []RuleId{
	RuleIdRemovableComment,
	RuleIdRedundantWhitespace,
	RuleIdRedundantRepeat,
	RuleIdNonCanonicalLiteral,
}

// formatFindings reports the rules that minification rewrites: those
// carrying whitespace or comments they do not need, or writing a repeat or a
// literal at greater length than its meaning requires. Everything outside a
// rule is a blank or comment line, which carries nothing at all.
func (linter *linter) formatFindings(path *abnf.Path) {
	offset := 0
	for _, rulePath := range items(path, nodeRule) {
		linter.blankRegion(offset, rulePath.Start)

		linter.pieces = nil
		linter.rule(rulePath)
		linter.ruleRegion(rulePath)

		offset = rulePath.End
	}
	linter.blankRegion(offset, len(linter.source.normalized))
}

// blankRegion reports on a stretch of a definition holding no rule, which is
// to say blank and comment lines. All of it goes.
func (linter *linter) blankRegion(start int, end int) {
	if start >= end {
		return
	}

	ruleId := RuleIdRedundantWhitespace
	if holdsComment(linter.source.normalized[:end], start) {
		ruleId = RuleIdRemovableComment
	}

	text := linter.source.text(start, end)

	linter.add(linter.source.finding(ruleId, start, end, describeRegion(ruleId, text, ""), replacement("")))
}

// ruleRegion gathers the pieces found in a rule into a single finding
// carrying the rule as minification writes it. A rule is the unit
// minification rewrites, so it is the unit to report on.
func (linter *linter) ruleRegion(path *abnf.Path) {
	start, end := path.Start, path.End
	if start >= end {
		return
	}

	text := linter.source.text(start, end)

	ruleId, minified, ok := linter.foldedRule(path)
	if !ok {
		if len(linter.pieces) == 0 {
			return
		}
		ruleId, minified = leadingRuleId(linter.pieces), applyPieces(text, start, linter.pieces)
	}

	if minified == text {
		return
	}

	linter.add(linter.source.finding(ruleId, start, end, describeRegion(ruleId, text, minified), replacement(minified)))
}

// foldedRule returns what minification leaves of a rule given more than
// once, which RFC 5234 Section 3.3 allows through "=/". The alternatives of
// every later definition are folded into the first, so the first carries the
// whole rule and the rest go away.
func (linter *linter) foldedRule(path *abnf.Path) (RuleId, string, bool) {
	rulename := item(path, nodeRulename)
	if rulename == nil {
		return "", "", false
	}

	paths := linter.rulePaths[strings.ToLower(linter.source.text(rulename.Start, rulename.End))]
	if len(paths) < 2 {
		return "", "", false
	}

	if paths[0] != path {
		return RuleIdIncrementalAlternative, "", true
	}

	rule := linter.grammar.Rule(linter.source.text(rulename.Start, rulename.End))
	if rule == nil {
		return "", "", false
	}

	return RuleIdIncrementalAlternative, minify.RenderRule(rule), true
}

// applyPieces rewrites the text of a region with every piece found in it.
func applyPieces(text string, offset int, pieces []*piece) string {
	ordered := slices.Clone(pieces)
	slices.SortStableFunc(ordered, func(a *piece, b *piece) int { return a.start - b.start })

	var sb strings.Builder
	written := 0
	for _, piece := range ordered {
		start, end := piece.start-offset, piece.end-offset
		if start < written || end > len(text) {
			// Pieces never overlap; ignore anything unexpected rather than
			// producing a rewrite that was never analysed.
			continue
		}
		sb.WriteString(text[written:start])
		sb.WriteString(piece.replacement)
		written = end
	}
	sb.WriteString(text[written:])

	return sb.String()
}

// leadingRuleId returns the check that says the most about a set of pieces.
func leadingRuleId(pieces []*piece) RuleId {
	for _, ruleId := range pieceRuleIds {
		for _, piece := range pieces {
			if piece.ruleId == ruleId {
				return ruleId
			}
		}
	}

	return RuleIdRedundantWhitespace
}

func describeRegion(ruleId RuleId, text string, minified string) string {
	if ruleId == RuleIdIncrementalAlternative && minified == "" {
		return fmt.Sprintf(
			"the alternatives of %q are folded into the definition the rule was first given",
			strings.TrimSuffix(text, crlf),
		)
	}

	if minified == "" {
		lines := strings.Count(text, "\n")
		what := "of whitespace"
		if ruleId == RuleIdRemovableComment {
			what = "of comments and whitespace"
		}
		return fmt.Sprintf("the %d lines %s here carry nothing, and minification removes them", lines, what)
	}

	return fmt.Sprintf(
		"the rule is written as %q where minification writes it as %q",
		strings.TrimSuffix(text, crlf),
		strings.TrimSuffix(minified, crlf),
	)
}

// rule finds the pieces of a rule, which RFC 5234 Section 4 defines as
// `rulename defined-as elements c-nl`.
func (linter *linter) rule(path *abnf.Path) {
	// Whatever follows the elements is the c-nl terminating the rule, which
	// may be a comment but only needs to be a line ending.
	if elements := item(path, nodeElements); elements != nil {
		linter.gap(elements.End, path.End, crlf)
	}

	linter.walk(path)
}

// walk finds the pieces of every construct of a path that holds whitespace
// of its own, descending into the whole tree. The constructs partition the
// bytes between them, so no two pieces can cover the same byte.
func (linter *linter) walk(path *abnf.Path) {
	switch path.MatchRule {
	case nodeDefinedAs:
		linter.definedAs(path)
	case nodeElements:
		// `elements = alternation *WSP`: the trailing whitespace goes.
		if alternation := item(path, nodeAlternation); alternation != nil {
			linter.gap(alternation.End, path.End, "")
		}
	case nodeAlternation:
		linter.alternation(path)
	case nodeConcatenation:
		linter.concatenation(path)
	case nodeGroup, nodeOption:
		linter.enclosed(path)
	case nodeRepeat:
		linter.repeat(path)
	case nodeCharVal:
		linter.charVal(path)
	case nodeNumVal:
		linter.numVal(path)
	}

	for _, subpath := range path.Subpaths {
		linter.walk(subpath)
	}
}

// definedAs finds the pieces of `defined-as = *c-wsp ("=" / "=/") *c-wsp`. A
// rulename cannot hold an "=" and no element starts with one, so neither
// side needs whitespace.
func (linter *linter) definedAs(path *abnf.Path) {
	operator := skipTrivia(linter.source.normalized[:path.End], path.Start)

	end := operator + 1
	if end < path.End && linter.source.normalized[end] == '/' {
		end++
	}

	linter.gap(path.Start, operator, "")
	linter.gap(end, path.End, "")
}

// alternation finds the pieces of
// `alternation = concatenation *(*c-wsp "/" *c-wsp concatenation)`. No
// element starts or ends with a "/", so neither side of a separator needs
// whitespace.
func (linter *linter) alternation(path *abnf.Path) {
	concatenations := items(path, nodeConcatenation)
	if len(concatenations) == 0 {
		return
	}

	for i, concatenation := range concatenations[:len(concatenations)-1] {
		next := concatenations[i+1]

		separator := skipTrivia(linter.source.normalized[:next.Start], concatenation.End)
		linter.gap(concatenation.End, separator, "")
		linter.gap(separator+1, next.Start, "")
	}
}

// concatenation finds the pieces of
// `concatenation = repetition *(1*c-wsp repetition)`. This separator is the
// one piece of whitespace ABNF requires, and one space is enough of it.
func (linter *linter) concatenation(path *abnf.Path) {
	repetitions := items(path, nodeRepetition)
	if len(repetitions) == 0 {
		return
	}

	for i, repetition := range repetitions[:len(repetitions)-1] {
		linter.gap(repetition.End, repetitions[i+1].Start, " ")
	}
}

// enclosed finds the pieces of a group or an option, which hold whitespace
// between their brackets and the alternation within.
func (linter *linter) enclosed(path *abnf.Path) {
	alternation := item(path, nodeAlternation)
	if alternation == nil {
		return
	}

	linter.gap(path.Start+1, alternation.Start, "")
	linter.gap(alternation.End, path.End-1, "")
}

// repeat finds a `repeat = 1*DIGIT / (*DIGIT "*" *DIGIT)` written at greater
// length than its meaning requires, such as the "1*1" of "1*1DIGIT".
func (linter *linter) repeat(path *abnf.Path) {
	text := linter.source.text(path.Start, path.End)

	canonical, ok := canonicalRepeat(text)
	if !ok || canonical == text {
		return
	}

	linter.piece(RuleIdRedundantRepeat, path.Start, path.End, canonical)
}

// canonicalRepeat returns the shortest repeat meaning the same as the given
// one, which is empty for a repeat of exactly one.
func canonicalRepeat(text string) (string, bool) {
	star := strings.IndexByte(text, '*')
	if star == -1 {
		return canonicalCount(text, true)
	}

	minimum, maximum := text[:star], text[star+1:]
	if minimum != "" && minimum == maximum {
		return canonicalCount(minimum, true)
	}

	if minimum != "" {
		canonical, ok := canonicalCount(minimum, false)
		if !ok {
			return "", false
		}
		minimum = canonical
	}
	if maximum != "" {
		canonical, ok := canonicalCount(maximum, false)
		if !ok {
			return "", false
		}
		maximum = canonical
	}

	return minimum + "*" + maximum, true
}

// canonicalCount returns the shortest spelling of a repeat count. An exact
// count of one, and a minimum of zero, are what a repetition means without
// them.
func canonicalCount(text string, exact bool) (string, bool) {
	count, err := strconv.Atoi(text)
	if err != nil {
		return "", false
	}

	if (exact && count == 1) || (!exact && count == 0) {
		return "", true
	}

	return strconv.Itoa(count), true
}

// charVal finds the explicit "%i" of RFC 7405, which says what a char-val
// already means.
func (linter *linter) charVal(path *abnf.Path) {
	if !strings.EqualFold(linter.source.text(path.Start, path.Start+2), "%i") {
		return
	}

	linter.piece(RuleIdNonCanonicalLiteral, path.Start, path.Start+2, "")
}

// numVal finds a num-val base written in uppercase, which means the same as
// the lowercase RFC 5234 writes.
func (linter *linter) numVal(path *abnf.Path) {
	base := path.Start + 1

	text := linter.source.text(base, base+1)
	if lowercase := strings.ToLower(text); lowercase != text {
		linter.piece(RuleIdNonCanonicalLiteral, base, base+1, lowercase)
	}
}

// gap finds the bytes between two constructs, which hold nothing but
// whitespace and comments, when they are not already what minification puts
// there.
func (linter *linter) gap(start int, end int, wanted string) {
	if start >= end || linter.source.text(start, end) == wanted {
		return
	}

	ruleId := RuleIdRedundantWhitespace
	if holdsComment(linter.source.normalized[:end], start) {
		ruleId = RuleIdRemovableComment
	}

	linter.piece(ruleId, start, end, wanted)
}

func (linter *linter) piece(ruleId RuleId, start int, end int, replacement string) {
	linter.pieces = append(linter.pieces, &piece{ruleId: ruleId, start: start, end: end, replacement: replacement})
}

// skipTrivia returns the offset just past the run of whitespace, comments
// and line endings that starts at the given offset. RFC 5234 allows all
// three wherever it allows `c-wsp`.
func skipTrivia(input []byte, offset int) int {
	for offset < len(input) {
		switch {
		case input[offset] == ' ' || input[offset] == '\t':
			offset++
		case input[offset] == '\r' && offset+1 < len(input) && input[offset+1] == '\n':
			offset += 2
		case input[offset] == ';':
			// A comment runs to the end of its line; VCHAR holds no CR, so
			// nothing within it can be mistaken for a line ending.
			for offset < len(input) && input[offset] != '\n' {
				offset++
			}
			if offset < len(input) {
				offset++
			}
		default:
			return offset
		}
	}

	return offset
}

// holdsComment reports whether the run of trivia starting at the given
// offset holds a comment.
func holdsComment(input []byte, offset int) bool {
	for offset < len(input) {
		switch {
		case input[offset] == ';':
			return true
		case input[offset] == ' ' || input[offset] == '\t':
			offset++
		case input[offset] == '\r' && offset+1 < len(input) && input[offset+1] == '\n':
			offset += 2
		default:
			return false
		}
	}

	return false
}

// items returns the descendants of a path that matched the given rule of the
// ABNF grammar of ABNF, in source order, without descending into a match.
// The nesting of the grammar makes these the direct constituents of the
// path: an alternation reaches no concatenation but its own, as any deeper
// one sits inside a concatenation of its own.
func items(path *abnf.Path, ruleName string) []*abnf.Path {
	var found []*abnf.Path

	var walk func(current *abnf.Path)
	walk = func(current *abnf.Path) {
		if current.MatchRule == ruleName {
			found = append(found, current)
			return
		}
		for _, subpath := range current.Subpaths {
			walk(subpath)
		}
	}
	for _, subpath := range path.Subpaths {
		walk(subpath)
	}

	return found
}

// item returns the first descendant of a path that matched the given rule,
// or nil.
func item(path *abnf.Path, ruleName string) *abnf.Path {
	if found := items(path, ruleName); len(found) != 0 {
		return found[0]
	}
	return nil
}
