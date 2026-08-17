package lint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/altshiftab/utils_go/pkg/abnf"
	"github.com/altshiftab/utils_go/pkg/abnf/minify"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// ruleIds returns the checks the findings answer to, in order.
func ruleIds(findings []*Finding) []RuleId {
	ids := make([]RuleId, 0, len(findings))
	for _, finding := range findings {
		ids = append(ids, finding.RuleId)
	}
	return ids
}

// applyFindings rewrites a definition with every fixable finding of the
// given categories, back to front so that the offsets stay valid.
func applyFindings(t *testing.T, input []byte, findings []*Finding, categories ...Category) []byte {
	t.Helper()

	applicable := make([]*Finding, 0, len(findings))
	for _, finding := range findings {
		rule := finding.Rule()
		if rule == nil || !finding.Fixable() || !slices.Contains(categories, rule.Category) {
			continue
		}
		applicable = append(applicable, finding)
	}

	slices.SortStableFunc(applicable, func(a *Finding, b *Finding) int {
		return b.Start.Offset - a.Start.Offset
	})

	output := input
	previousStart := len(input)
	for _, finding := range applicable {
		if finding.End.Offset > previousStart {
			t.Fatalf(
				"findings overlap: %s at %d-%d reaches past %d",
				finding.RuleId,
				finding.Start.Offset,
				finding.End.Offset,
				previousStart,
			)
		}
		previousStart = finding.Start.Offset

		rewritten := make([]byte, 0, len(output))
		rewritten = append(rewritten, output[:finding.Start.Offset]...)
		rewritten = append(rewritten, *finding.Replacement...)
		rewritten = append(rewritten, output[finding.End.Offset:]...)
		output = rewritten
	}

	return output
}

func TestLint(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		simplify bool
		expected []RuleId
	}{
		{
			name:  "minified definition is clean",
			input: "root=\"a\"/\"b\"\r\nother=root\r\n",
			// The root rule is the one nothing refers to.
			expected: []RuleId{RuleIdUnreferencedRule},
		},
		{
			name:     "redundant whitespace",
			input:    "root = \"a\" / \"b\"\r\nother = root\r\n",
			expected: []RuleId{RuleIdRedundantWhitespace, RuleIdRedundantWhitespace, RuleIdUnreferencedRule},
		},
		{
			name:     "comment on a rule",
			input:    "root=\"a\" ; why\r\n",
			expected: []RuleId{RuleIdRemovableComment, RuleIdUnreferencedRule},
		},
		{
			name:     "comment on its own line",
			input:    "; why\r\nroot=\"a\"\r\n",
			expected: []RuleId{RuleIdRemovableComment, RuleIdUnreferencedRule},
		},
		{
			name:     "blank line",
			input:    "root=\"a\"\r\n\r\n",
			expected: []RuleId{RuleIdUnreferencedRule, RuleIdRedundantWhitespace},
		},
		{
			name:     "lf line endings",
			input:    "root=\"a\"\nother=root\n",
			expected: []RuleId{RuleIdNonCrlfLineEnding, RuleIdUnreferencedRule, RuleIdNonCrlfLineEnding},
		},
		{
			name:     "unterminated last line",
			input:    "root=\"a\"\r\nother=root",
			expected: []RuleId{RuleIdUnreferencedRule, RuleIdNonCrlfLineEnding},
		},
		{
			name:     "redundant repeat",
			input:    "root=1*1DIGIT\r\n",
			expected: []RuleId{RuleIdRedundantRepeat, RuleIdUnreferencedRule},
		},
		{
			name:     "explicit case-insensitive marker",
			input:    "root=%i\"a\"\r\n",
			expected: []RuleId{RuleIdNonCanonicalLiteral, RuleIdUnreferencedRule},
		},
		{
			name:     "uppercase num-val base",
			input:    "root=%X41\r\n",
			expected: []RuleId{RuleIdNonCanonicalLiteral, RuleIdUnreferencedRule},
		},
		{
			name:     "prose-val never matches",
			input:    "root=<a description>\r\n",
			expected: []RuleId{RuleIdUnreferencedRule, RuleIdUnmatchableProseVal},
		},
		{
			name:     "duplicate alternative",
			input:    "root=\"a\"/\"b\"/\"a\"\r\n",
			expected: []RuleId{RuleIdDuplicateAlternative, RuleIdUnreferencedRule},
		},
		{
			name:     "inconsistent rulename case",
			input:    "root=Other\r\nother=\"a\"\r\n",
			expected: []RuleId{RuleIdUnreferencedRule, RuleIdInconsistentRulenameCase},
		},
		{
			name:     "inconsistent core rulename case",
			input:    "root=alpha\r\n",
			expected: []RuleId{RuleIdUnreferencedRule, RuleIdInconsistentRulenameCase},
		},
		{
			name:     "no simplification without the option",
			input:    "root=(\"a\")\r\n",
			expected: []RuleId{RuleIdUnreferencedRule},
		},
		{
			name:     "redundant group",
			input:    "root=(\"a\"/\"b\")\r\n",
			simplify: true,
			expected: []RuleId{RuleIdRedundantGroup, RuleIdUnreferencedRule},
		},
		{
			name:     "verbose num-val",
			input:    "root=%x4D.6F.6E\r\n",
			simplify: true,
			expected: []RuleId{RuleIdVerboseNumVal, RuleIdUnreferencedRule},
		},
		{
			name:     "joinable literals",
			input:    "root=\"a\" \"b\"\r\n",
			simplify: true,
			expected: []RuleId{RuleIdJoinableLiterals, RuleIdUnreferencedRule},
		},
		{
			name:  "incremental alternative is folded",
			input: "root=\"a\"\r\nother=root\r\nroot=/\"b\"\r\n",
			expected: []RuleId{
				RuleIdIncrementalAlternative,
				RuleIdUnreferencedRule,
				RuleIdIncrementalAlternative,
			},
		},
		{
			name:     "list extension",
			input:    "root=1#token\r\ntoken=ALPHA\r\nOWS=*(SP/HTAB)\r\n",
			expected: []RuleId{RuleIdListExtension, RuleIdUnreferencedRule},
		},
		{
			name:     "hash inside a char-val is not the list extension",
			input:    "root=\"#\"\r\n",
			expected: []RuleId{RuleIdUnreferencedRule},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			findings, err := Lint([]byte(testCase.input), &Options{Simplify: testCase.simplify})
			if err != nil {
				t.Fatalf("lint: %v", err)
			}

			if ids := ruleIds(findings); !slices.Equal(ids, testCase.expected) {
				t.Fatalf("expected %v, got %v", testCase.expected, ids)
			}
		})
	}
}

// TestUnreachableRules checks the check that naming the rules a grammar is
// parsed from makes possible: a rule nothing refers to is only suspicious,
// but a rule no root leads to is dead.
func TestUnreachableRules(t *testing.T) {
	t.Parallel()

	// "dead" is referred to by "also-dead", so counting references calls it
	// used; nothing the grammar is parsed from leads to either.
	const definition = "root=live\r\n" +
		"live=\"a\"\r\n" +
		"also-dead=dead\r\n" +
		"dead=\"b\"\r\n"

	testCases := []struct {
		name     string
		roots    []string
		expected []RuleId
	}{
		{
			name: "without roots only references are counted",
			// "dead" goes unreported: "also-dead" refers to it.
			expected: []RuleId{RuleIdUnreferencedRule, RuleIdUnreferencedRule},
		},
		{
			name:  "with a root everything it does not reach is dead",
			roots: []string{"root"},
			expected: []RuleId{
				RuleIdUnreachableRule,
				RuleIdUnreachableRule,
			},
		},
		{
			name:     "naming every entry point leaves nothing dead",
			roots:    []string{"root", "also-dead"},
			expected: nil,
		},
		{
			name:     "a root is matched without regard to case",
			roots:    []string{"ROOT", "also-dead"},
			expected: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			findings, err := Lint([]byte(definition), &Options{Roots: testCase.roots})
			if err != nil {
				t.Fatalf("lint: %v", err)
			}

			if ids := ruleIds(findings); !slices.Equal(ids, testCase.expected) {
				t.Fatalf("expected %v, got %v", testCase.expected, ids)
			}
		})
	}
}

// TestUnknownRootIsReported checks that naming a rule the grammar does not
// hold is an error rather than a silent lack of findings.
func TestUnknownRootIsReported(t *testing.T) {
	t.Parallel()

	_, err := Lint([]byte("root=\"a\"\r\n"), &Options{Roots: []string{"missing"}})
	if err == nil {
		t.Fatal("expected an error")
	}

	notFound, ok := errors.AsType[*abnf.RuleNotFoundError](err)
	if !ok {
		t.Fatalf("expected a rule-not-found error, got: %v", err)
	}
	if notFound.Rulename != "missing" {
		t.Fatalf("expected the missing rule to be named, got %q", notFound.Rulename)
	}
}

func TestLintErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
	}{
		{name: "empty input"},
		{name: "not a grammar", input: "not a grammar"},
		{name: "missing dependency", input: "root = missing-rule\r\n"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := Lint([]byte(testCase.input), nil); !errors.Is(err, altshiftErrors.ErrParseError) {
				t.Fatalf("expected a parse error, got: %v", err)
			}
		})
	}
}

// TestFindingsAgreeWithMinification checks the promise the formatting checks
// make: acting on every one of them leaves exactly what minification writes.
// It is what keeps the linter and the minifier from drifting apart.
func TestFindingsAgreeWithMinification(t *testing.T) {
	t.Parallel()

	inputs := append(
		[]string{
			"; a comment\r\n\r\nroot = ( \"a\" / \"b\" )  1*1( DIGIT ) ; why\r\n\r\nother  =\troot\r\n",
			"root = \"a\" /\r\n  \"b\"\r\n",
			"root=%i\"a\" %X41\r\n",
			"root = \"a\"\nother = root\n",
			"root = \"a\"\r\nother = root",
			"root=\"a\"\r\nroot =/ \"b\"\r\nother = root\r\n",
		},
		grammarTexts(t)...,
	)

	for i, input := range inputs {
		t.Run(fmt.Sprintf("input %d", i), func(t *testing.T) {
			t.Parallel()

			findings, err := Lint([]byte(input), nil)
			if err != nil {
				t.Fatalf("lint: %v", err)
			}

			applied := applyFindings(t, []byte(input), findings, CategoryFormatting)

			minified, err := minify.Minify([]byte(input), nil)
			if err != nil {
				t.Fatalf("minify: %v", err)
			}

			if string(applied) != string(minified) {
				t.Fatalf("acting on the findings gave %q, minification gives %q", string(applied), string(minified))
			}
		})
	}
}

// TestMinifiedDefinitionsAreClean checks that a minified definition draws no
// formatting finding, and a simplified one no simplification finding.
func TestMinifiedDefinitionsAreClean(t *testing.T) {
	t.Parallel()

	for i, input := range grammarTexts(t) {
		t.Run(fmt.Sprintf("grammar %d", i), func(t *testing.T) {
			t.Parallel()

			options := &minify.Options{Simplify: true}

			minified, err := minify.Minify([]byte(input), options)
			if err != nil {
				t.Fatalf("minify: %v", err)
			}

			findings, err := Lint(minified, &Options{Simplify: true})
			if err != nil {
				t.Fatalf("lint: %v", err)
			}

			for _, finding := range findings {
				if category := finding.Rule().Category; category != CategoryQuality {
					t.Fatalf(
						"a minified definition still draws %s (%s): %s",
						finding.RuleId,
						category,
						finding.Message,
					)
				}
			}
		})
	}
}

func TestCanonicalRepeat(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		input     string
		expected  string
		expectsOk bool
	}{
		{name: "exact one", input: "1", expected: "", expectsOk: true},
		{name: "exact two", input: "2", expected: "2", expectsOk: true},
		{name: "bounded one", input: "1*1", expected: "", expectsOk: true},
		{name: "bounded two", input: "2*2", expected: "2", expectsOk: true},
		{name: "zero minimum", input: "0*3", expected: "*3", expectsOk: true},
		{name: "unbounded", input: "*", expected: "*", expectsOk: true},
		{name: "minimum only", input: "1*", expected: "1*", expectsOk: true},
		{name: "maximum only", input: "*3", expected: "*3", expectsOk: true},
		{name: "already shortest", input: "2*4", expected: "2*4", expectsOk: true},
		{name: "leading zeroes", input: "01*03", expected: "1*3", expectsOk: true},
		{name: "not a repeat", input: "x", expectsOk: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			canonical, ok := canonicalRepeat(testCase.input)
			if ok != testCase.expectsOk {
				t.Fatalf("expected ok=%t, got ok=%t", testCase.expectsOk, ok)
			}
			if ok && canonical != testCase.expected {
				t.Fatalf("expected %q, got %q", testCase.expected, canonical)
			}
		})
	}
}

func TestPositions(t *testing.T) {
	t.Parallel()

	// The prose-val starts on the second line, at the ninth character.
	findings, err := Lint([]byte("root=other\r\nother=<why>\r\n"), nil)
	if err != nil {
		t.Fatalf("lint: %v", err)
	}

	var proseVal *Finding
	for _, finding := range findings {
		if finding.RuleId == RuleIdUnmatchableProseVal {
			proseVal = finding
		}
	}
	if proseVal == nil {
		t.Fatal("expected a prose-val finding")
	}

	if proseVal.Start.Line != 2 || proseVal.Start.Column != 7 {
		t.Fatalf("expected the finding at 2:7, got %d:%d", proseVal.Start.Line, proseVal.Start.Column)
	}
	if proseVal.End.Offset-proseVal.Start.Offset != 5 {
		t.Fatalf("expected the finding to cover 5 bytes, got %d", proseVal.End.Offset-proseVal.Start.Offset)
	}
}

// grammarTexts returns the ABNF grammar definitions of the module, which
// serve as real-world lint input.
func grammarTexts(t *testing.T) []string {
	t.Helper()

	var paths []string
	for _, pattern := range []string{
		filepath.Join("..", "..", "..", "pkg", "*", "*", "grammar.abnf"),
		filepath.Join("..", "..", "..", "pkg", "*", "*", "*", "grammar.abnf"),
	} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("filepath glob: %v", err)
		}
		paths = append(paths, matches...)
	}

	if len(paths) == 0 {
		t.Fatal("no grammar definitions were found")
	}

	texts := make([]string, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("os read file: %v", err)
		}
		texts = append(texts, string(data))
	}

	return texts
}
