package lint

import (
	"github.com/altshiftab/utils_go/pkg/sarif"
)

// RuleId identifies a check the linter performs.
type RuleId string

const (
	// RuleIdRedundantWhitespace marks whitespace that carries no meaning.
	// RFC 5234 allows it nearly everywhere; only the separator between the
	// repetitions of a concatenation has to stay, and only as one space.
	RuleIdRedundantWhitespace RuleId = "redundant-whitespace"
	// RuleIdRemovableComment marks a comment, which a grammar definition
	// does not need to carry.
	RuleIdRemovableComment RuleId = "removable-comment"
	// RuleIdNonCrlfLineEnding marks a line that does not end with the CRLF
	// that RFC 5234 requires, including a missing final one.
	RuleIdNonCrlfLineEnding RuleId = "non-crlf-line-ending"
	// RuleIdRedundantRepeat marks a repeat that says more than it needs to,
	// such as the "1*1" of "1*1DIGIT" or the leading zero of "0*3DIGIT".
	RuleIdRedundantRepeat RuleId = "redundant-repeat"
	// RuleIdNonCanonicalLiteral marks a literal written in a longer form
	// than it needs: the explicit "%i" of RFC 7405, which is the default,
	// or an uppercase num-val base.
	RuleIdNonCanonicalLiteral RuleId = "non-canonical-literal"
	// RuleIdIncrementalAlternative marks a rule extended with the "=/" of
	// RFC 5234 Section 3.3, which minification folds into the definition
	// the rule was first given.
	RuleIdIncrementalAlternative RuleId = "incremental-alternative"

	// RuleIdRedundantGroup marks a rule holding parentheses that can be
	// dropped without changing what it matches.
	RuleIdRedundantGroup RuleId = "redundant-group"
	// RuleIdVerboseNumVal marks a rule holding a num-val that a shorter
	// char-val expresses just as exactly.
	RuleIdVerboseNumVal RuleId = "verbose-num-val"
	// RuleIdJoinableLiterals marks a rule holding adjacent literals that a
	// single literal can express.
	RuleIdJoinableLiterals RuleId = "joinable-literals"

	// RuleIdUnmatchableProseVal marks a prose-val, which is a description
	// for a human reader and matches no input at all.
	RuleIdUnmatchableProseVal RuleId = "unmatchable-prose-val"
	// RuleIdDuplicateAlternative marks an alternation offering the same
	// alternative more than once, where the later ones are dead.
	RuleIdDuplicateAlternative RuleId = "duplicate-alternative"
	// RuleIdInconsistentRulenameCase marks a reference that spells a rule
	// name with a different case than the rule was defined with. Rule names
	// are case-insensitive, so the two mean the same rule.
	RuleIdInconsistentRulenameCase RuleId = "inconsistent-rulename-case"
	// RuleIdListExtension marks a rule using the "#" list operator of
	// RFC 9110 Section 5.6.1. pkg/abnf reads it, but it belongs to the HTTP
	// specifications rather than to RFC 5234, so a definition holding one is
	// not portable to a parser that reads RFC 5234 alone.
	RuleIdListExtension RuleId = "list-extension"
	// RuleIdUnreferencedRule marks a rule that no other rule refers to.
	// That is expected of the rule a grammar is parsed from, and suspicious
	// of any other. Naming the rules a grammar is parsed from, through
	// Options.Roots, answers the question far better: see
	// RuleIdUnreachableRule.
	RuleIdUnreferencedRule RuleId = "unreferenced-rule"
	// RuleIdUnreachableRule marks a rule that no rule a grammar is parsed
	// from leads to, directly or through others. Such a rule is dead: no
	// input can ever reach it.
	RuleIdUnreachableRule RuleId = "unreachable-rule"
)

// Category groups the checks by what acting on them achieves.
type Category string

const (
	// CategoryFormatting covers findings that minification removes. Acting
	// on them leaves the grammar itself untouched.
	CategoryFormatting Category = "formatting"
	// CategorySimplification covers findings that simplification rewrites.
	// Acting on them preserves the matched language, but changes the shape
	// of the paths that parsing produces.
	CategorySimplification Category = "simplification"
	// CategoryQuality covers findings that report on the grammar itself
	// rather than on how it is written.
	CategoryQuality Category = "quality"
)

// Rule describes a check the linter performs.
type Rule struct {
	Id          RuleId
	Category    Category
	Level       sarif.Level
	Description string
}

// rules describes every check, in reporting order.
var rules = []*Rule{
	{
		Id:          RuleIdRedundantWhitespace,
		Category:    CategoryFormatting,
		Level:       sarif.LevelNote,
		Description: "Whitespace that RFC 5234 does not require, and that minification removes.",
	},
	{
		Id:          RuleIdRemovableComment,
		Category:    CategoryFormatting,
		Level:       sarif.LevelNote,
		Description: "A comment, which minification removes.",
	},
	{
		Id:          RuleIdNonCrlfLineEnding,
		Category:    CategoryFormatting,
		Level:       sarif.LevelWarning,
		Description: "A line ending that is not the CRLF that RFC 5234 requires.",
	},
	{
		Id:          RuleIdRedundantRepeat,
		Category:    CategoryFormatting,
		Level:       sarif.LevelNote,
		Description: "A repeat written in a longer form than the one it means.",
	},
	{
		Id:          RuleIdNonCanonicalLiteral,
		Category:    CategoryFormatting,
		Level:       sarif.LevelNote,
		Description: "A literal written in a longer form than the one it means.",
	},
	{
		Id:          RuleIdIncrementalAlternative,
		Category:    CategoryFormatting,
		Level:       sarif.LevelNote,
		Description: "A rule extended with \"=/\", which minification folds into the definition it extends.",
	},
	{
		Id:          RuleIdRedundantGroup,
		Category:    CategorySimplification,
		Level:       sarif.LevelNote,
		Description: "Parentheses that can be dropped without changing what the rule matches.",
	},
	{
		Id:          RuleIdVerboseNumVal,
		Category:    CategorySimplification,
		Level:       sarif.LevelNote,
		Description: "A num-val that a shorter char-val expresses just as exactly.",
	},
	{
		Id:          RuleIdJoinableLiterals,
		Category:    CategorySimplification,
		Level:       sarif.LevelNote,
		Description: "Adjacent literals that a single literal can express.",
	},
	{
		Id:          RuleIdUnmatchableProseVal,
		Category:    CategoryQuality,
		Level:       sarif.LevelWarning,
		Description: "A prose-val, which describes what to match instead of matching it, and so matches nothing.",
	},
	{
		Id:          RuleIdDuplicateAlternative,
		Category:    CategoryQuality,
		Level:       sarif.LevelWarning,
		Description: "An alternative offered more than once in the same alternation, where the later ones are dead.",
	},
	{
		Id:          RuleIdInconsistentRulenameCase,
		Category:    CategoryQuality,
		Level:       sarif.LevelNote,
		Description: "A reference spelling a rule name with a different case than its definition.",
	},
	{
		Id:          RuleIdListExtension,
		Category:    CategoryQuality,
		Level:       sarif.LevelNote,
		Description: "A use of the \"#\" list operator of RFC 9110, which RFC 5234 alone does not define.",
	},
	{
		Id:          RuleIdUnreferencedRule,
		Category:    CategoryQuality,
		Level:       sarif.LevelNote,
		Description: "A rule that no other rule refers to, which is expected only of a root rule.",
	},
	{
		Id:          RuleIdUnreachableRule,
		Category:    CategoryQuality,
		Level:       sarif.LevelWarning,
		Description: "A rule that no root rule leads to, which no input can ever reach.",
	},
}

// Rules returns every check the linter performs, in reporting order.
func Rules() []*Rule {
	return rules
}

// ruleById indexes the checks by identifier.
var ruleById = func() map[RuleId]*Rule {
	byId := make(map[RuleId]*Rule, len(rules))
	for _, rule := range rules {
		byId[rule.Id] = rule
	}
	return byId
}()

// Position is a location in a grammar definition, counted over the bytes of
// the definition as given, before any line-ending normalization.
type Position struct {
	// Offset is the zero-based byte offset.
	Offset int
	// Line and Column are one-based, as editors and SARIF count them.
	Line, Column int
}

// Finding is one reported occurrence of a check.
type Finding struct {
	RuleId RuleId
	// Start and End delimit the bytes the finding covers; Start <= End.
	Start, End *Position
	// Message says what is wrong, in one sentence.
	Message string
	// Replacement is the text that the bytes from Start to End should be
	// replaced with to act on the finding. It is empty for a deletion, and
	// nil for a finding the linter cannot act on.
	Replacement *string
}

// Rule returns the check the finding reports on.
func (finding *Finding) Rule() *Rule {
	return ruleById[finding.RuleId]
}

// Fixable reports whether the linter knows how to act on the finding.
func (finding *Finding) Fixable() bool {
	return finding.Replacement != nil
}
