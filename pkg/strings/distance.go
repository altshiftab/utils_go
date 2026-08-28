package strings

// Levenshtein is the edit distance between two rune slices: the fewest single-rune
// insertions, deletions and substitutions that turn one into the other.
//
// Named for the metric rather than called an edit distance, because several
// distances answer to that description and they disagree. Damerau-Levenshtein
// counts a transposition as one edit where this counts two, and a caller
// comparing text a human typed usually wants the former.
//
// Computed with two rows rather than the full matrix. The inputs this was written
// for are short — a sentence, a paragraph — but there are a great many of them,
// and the allocation is what would be felt rather than the arithmetic.
func Levenshtein(a []rune, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			substitution := previous[j-1]
			if a[i-1] != b[j-1] {
				substitution++
			}
			current[j] = min(min(previous[j]+1, current[j-1]+1), substitution)
		}
		previous, current = current, previous
	}

	return previous[len(b)]
}
