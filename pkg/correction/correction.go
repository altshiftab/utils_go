// Package correction decides whether a proposed correction to a text is small
// enough to be one.
//
// A model asked to correct a text will, given the chance, improve it — tidying
// grammar, trimming repetition, replacing a word with a likelier one. Every one
// of those reads as a correction and none of them is: what comes back is no
// longer the writer's text. The measures here are the backstop, applied to each
// proposed correction before it is shown or accepted, so that the model's
// willingness to rewrite is bounded by something other than its instructions.
package correction

import (
	"strings"
	"unicode"

	altshiftStrings "github.com/altshiftab/utils_go/pkg/strings"
)

const (
	// MaximumChangeRatio is how much of a text, as written, a correction may
	// alter. A coarse backstop against a wholesale rewrite, and against a
	// correction landing against the wrong index — a swap that would put one
	// paragraph's words where another's belong.
	MaximumChangeRatio = 0.25

	// MaximumLetterChangeRatio is the one that does the work: how much of a
	// text's letters — what is left once spacing and punctuation are set aside
	// — a correction may alter.
	//
	// Almost every real error in written Swedish is a compound split in the
	// wrong place: "Riks banken", "procent enhet", "finans departementet".
	// Repairing one moves no letters at all, only the spaces between them, so a
	// guard that looks at letters alone lets every such fix straight through
	// while a substituted word stands out immediately.
	//
	// The distinction is not academic. Asked to correct "Molntjänster och bo
	// städer", a model returned "Miljarder till bostäder" — it did not join the
	// split compound, it replaced the words with likelier ones and left the
	// meaning somewhere else entirely. Measured over the text as written that
	// came to 21%, comfortably inside the ratio above; measured over the letters
	// it is a quarter of them, and nothing like a spelling fix.
	MaximumLetterChangeRatio = 0.10

	// MinimumLetterChange is a floor under the ratio, so that a short text can
	// still have a misspelling corrected rather than being immune to correction
	// for being short.
	MinimumLetterChange = 3
)

// Acceptable reports whether a correction is small enough to be one.
//
// Two measures, because they catch different failures. Distance over the text as
// written catches a wholesale rewrite. Distance over the letters alone catches a
// model that answered with likelier words rather than the same words spelled
// right, which reads as a correction and is not one.
//
// An empty original is not acceptable to correct: there is nothing to measure
// against, so anything at all would pass both ratios.
func Acceptable(original string, corrected string) bool {
	if original == "" {
		return false
	}

	originalRunes := []rune(original)
	if altshiftStrings.Levenshtein(originalRunes, []rune(corrected)) >
		int(float64(len(originalRunes))*MaximumChangeRatio) {
		return false
	}

	originalLetters := Letters(original)
	allowance := max(MinimumLetterChange, int(float64(len(originalLetters))*MaximumLetterChangeRatio))

	return altshiftStrings.Levenshtein(originalLetters, Letters(corrected)) <= allowance
}

// Letters reduces a text to the characters carrying its words, so that two texts
// differing only in how those words are split, cased or punctuated compare as
// equal. Joining a compound that was broken moves no letters at all.
func Letters(text string) []rune {
	var result []rune
	for _, character := range strings.ToLower(text) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			result = append(result, character)
		}
	}

	return result
}
