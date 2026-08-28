package strings

import "testing"

func TestLevenshtein(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		a        string
		b        string
		expected int
	}{
		{name: "identical", a: "kitten", b: "kitten", expected: 0},
		{name: "both empty", a: "", b: "", expected: 0},
		{name: "empty against a word is its length", a: "", b: "kitten", expected: 6},
		{name: "a word against empty is its length", a: "kitten", b: "", expected: 6},
		{name: "the canonical example", a: "kitten", b: "sitting", expected: 3},
		{name: "one substitution", a: "peice", b: "peace", expected: 1},
		{name: "one insertion", a: "hous", b: "house", expected: 1},
		{name: "one deletion", a: "housee", b: "house", expected: 1},
		// A transposition is two edits here, not one. That is the difference
		// between this and Damerau-Levenshtein, and it is asserted so that
		// swapping the implementation for one cannot go unnoticed.
		{name: "a transposition costs two", a: "teh", b: "the", expected: 2},
		// Runes, not bytes: these differ by one character but by two bytes.
		{name: "non-ascii counts runes", a: "hök", b: "hok", expected: 1},
		{name: "swedish compound rejoined", a: "bo städer", b: "bostäder", expected: 1},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if distance := Levenshtein([]rune(testCase.a), []rune(testCase.b)); distance != testCase.expected {
				t.Errorf("expected %d, got %d", testCase.expected, distance)
			}
		})
	}
}

// The distance does not depend on which way round it is asked.
func TestLevenshteinIsSymmetric(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		a    string
		b    string
	}{
		{name: "differing lengths", a: "kitten", b: "sitting"},
		{name: "one empty", a: "", b: "hus"},
		{name: "non-ascii", a: "vår höst", b: "var host"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			forward := Levenshtein([]rune(testCase.a), []rune(testCase.b))
			backward := Levenshtein([]rune(testCase.b), []rune(testCase.a))

			if forward != backward {
				t.Errorf("expected the same distance both ways, got %d and %d", forward, backward)
			}
		})
	}
}
