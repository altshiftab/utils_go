// Package lint reports on ABNF grammar definitions (RFC 5234, with the
// RFC 7405 char-val extension and the RFC 9110 Section 5.6.1 "#" list
// operator): on the whitespace and comments that
// minification removes, on the expressions that simplification shortens, and
// on the grammar itself.
//
// Findings carry the bytes of the definition they cover, so that they can be
// reported against the source, and the text that acting on one puts in their
// place. Sarif renders them as a SARIF 2.1.0 log.
package lint

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/altshiftab/utils_go/pkg/abnf"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// Options holds the settings of a lint run.
type Options struct {
	// Simplify includes the checks of CategorySimplification, which report
	// expressions that can be written more shortly without changing what
	// they match. They are left out by default because acting on them
	// changes the shape of the paths that parsing produces.
	Simplify bool
	// Roots names the rules the grammar is parsed from. Given them, the
	// linter reports the rules none of them leads to, which are dead;
	// without them it can only report the rules nothing refers to, which
	// every root rule is by definition.
	Roots []string
}

// linter accumulates the findings of one run over one definition.
type linter struct {
	source  *source
	grammar *abnf.Grammar
	// rulePaths indexes the definitions of each rule, by lowercase rule
	// name. A rule extended with the "=/" of RFC 5234 Section 3.3 has more
	// than one.
	rulePaths map[string][]*abnf.Path
	// pieces holds what was found in the rule being looked at, before it is
	// gathered into a finding.
	pieces   []*piece
	findings []*Finding
}

func (linter *linter) add(finding *Finding) {
	if finding != nil {
		linter.findings = append(linter.findings, finding)
	}
}

// Lint reports on an ABNF grammar definition. Definitions using LF line
// endings, which RFC 5234 does not allow, are reported and then read as
// though they used CRLF, so that the rest of the checks still run.
//
// The "#" list operator of RFC 9110 Section 5.6.1 is read as the standard
// ABNF it expands to, and reported so that minification writing it out does
// not come as a surprise.
func Lint(input []byte, options *Options) ([]*Finding, error) {
	source := newSource(input)

	grammar, path, err := abnf.ParseABNFWithPath(source.normalized)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: abnf parse abnf with path: %w",
				altshiftErrors.ErrParseError,
				explainListSeparator(source.normalized, err),
			),
		)
	}

	linter := &linter{source: source, grammar: grammar, rulePaths: map[string][]*abnf.Path{}}
	for _, rulePath := range items(path, nodeRule) {
		rulename := item(rulePath, nodeRulename)
		if rulename == nil {
			continue
		}
		name := strings.ToLower(source.text(rulename.Start, rulename.End))
		linter.rulePaths[name] = append(linter.rulePaths[name], rulePath)
	}

	linter.lineEndingFindings()
	linter.formatFindings(path)
	if options != nil && options.Simplify {
		linter.simplificationFindings()
	}
	if err := linter.qualityFindings(path, options); err != nil {
		return nil, fmt.Errorf("quality findings: %w", err)
	}

	return sortFindings(suppressCoveredLineEndings(linter.findings)), nil
}

// lineEndingFindings reports the lines that do not end with the CRLF that
// RFC 5234 requires, the last one included.
func (linter *linter) lineEndingFindings() {
	original := linter.source.original

	for i, b := range original {
		if b != '\n' || (i != 0 && original[i-1] == '\r') {
			continue
		}

		linter.add(
			linter.source.originalFinding(
				RuleIdNonCrlfLineEnding,
				i,
				i+1,
				"the line ends with a bare LF, where RFC 5234 ends every line with CRLF",
				replacement(crlf),
			),
		)
	}

	if length := len(original); length != 0 && !bytes.HasSuffix(original, []byte("\n")) {
		linter.add(
			linter.source.originalFinding(
				RuleIdNonCrlfLineEnding,
				length,
				length,
				"the last line has no line ending, where RFC 5234 ends every line with CRLF",
				replacement(crlf),
			),
		)
	}
}

// suppressCoveredLineEndings drops the line-ending findings that another
// finding already covers, which happens wherever a blank line or a folded
// expression goes away as a whole.
func suppressCoveredLineEndings(findings []*Finding) []*Finding {
	kept := make([]*Finding, 0, len(findings))

	for _, finding := range findings {
		if finding.RuleId == RuleIdNonCrlfLineEnding && coveredBy(finding, findings) {
			continue
		}
		kept = append(kept, finding)
	}

	return kept
}

func coveredBy(finding *Finding, findings []*Finding) bool {
	for _, other := range findings {
		if other == finding || other.RuleId == RuleIdNonCrlfLineEnding || !other.Fixable() {
			continue
		}
		if other.Start.Offset <= finding.Start.Offset && finding.End.Offset <= other.End.Offset {
			return true
		}
	}

	return false
}

// sortFindings orders findings by where they are, and then by the order the
// checks are declared in, so that a run reports the same thing every time.
func sortFindings(findings []*Finding) []*Finding {
	ruleOrder := make(map[RuleId]int, len(rules))
	for i, rule := range rules {
		ruleOrder[rule.Id] = i
	}

	slices.SortStableFunc(findings, func(a *Finding, b *Finding) int {
		if a.Start.Offset != b.Start.Offset {
			return a.Start.Offset - b.Start.Offset
		}
		return ruleOrder[a.RuleId] - ruleOrder[b.RuleId]
	})

	return findings
}
