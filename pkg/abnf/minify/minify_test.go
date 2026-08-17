package minify

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/abnf"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

func TestMinify(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		simplify bool
		expected string
	}{
		{
			name:     "whitespace around defined-as and alternation",
			input:    "root  =  \"a\"  /  \"b\"\r\n",
			expected: "root=\"a\"/\"b\"\r\n",
		},
		{
			name:     "concatenation separator kept",
			input:    "root = \"a\"    \"b\"\r\n",
			expected: "root=\"a\" \"b\"\r\n",
		},
		{
			name:     "whitespace inside groups and options",
			input:    "root = ( \"a\" / \"b\" ) [ \"c\" ]\r\n",
			expected: "root=(\"a\"/\"b\") [\"c\"]\r\n",
		},
		{
			name:     "comments and empty lines",
			input:    "; a comment\r\n\r\nroot = \"a\" ; a trailing comment\r\n\r\n",
			expected: "root=\"a\"\r\n",
		},
		{
			name:     "line folding",
			input:    "root = \"a\" /\r\n  \"b\"\r\n",
			expected: "root=\"a\"/\"b\"\r\n",
		},
		{
			name:     "lf line endings",
			input:    "root = \"a\"\nother = root\n",
			expected: "root=\"a\"\r\nother=root\r\n",
		},
		{
			name:     "unterminated last line",
			input:    "root = \"a\"",
			expected: "root=\"a\"\r\n",
		},
		{
			name:     "definition order kept",
			input:    "zulu = \"z\"\r\nalpha-rule = zulu\r\nmike = alpha-rule\r\n",
			expected: "zulu=\"z\"\r\nalpha-rule=zulu\r\nmike=alpha-rule\r\n",
		},
		{
			name:     "incremental alternatives merged",
			input:    "root = \"a\"\r\nroot =/ \"b\"\r\nother = root\r\n",
			expected: "root=\"a\"/\"b\"\r\nother=root\r\n",
		},
		{
			name:     "repeats normalized",
			input:    "root = 1*1\"a\" 0*\"b\" 2*2\"c\" 1*3\"d\"\r\n",
			expected: "root=\"a\" *\"b\" 2\"c\" 1*3\"d\"\r\n",
		},
		{
			name:     "elements kept verbatim",
			input:    "root = %s\"aB\" %x0D.0A <a prose value> \"a / b\"\r\n",
			expected: "root=%s\"aB\" %x0D.0A <a prose value> \"a / b\"\r\n",
		},
		{
			name:     "no simplification without the option",
			input:    "root = (\"a\") / (\"b\" / \"c\")\r\n",
			expected: "root=(\"a\")/(\"b\"/\"c\")\r\n",
		},
		{
			name:     "simplify parenthesised alternatives",
			input:    "root = (\"a\") / (\"b\" / \"c\")\r\n",
			simplify: true,
			expected: "root=\"a\"/\"b\"/\"c\"\r\n",
		},
		{
			name:     "simplify group in concatenation",
			input:    "root = \"a\" (\"b\" \"c\") \"d\"\r\n",
			simplify: true,
			expected: "root=\"abcd\"\r\n",
		},
		{
			name:     "simplify repeated group of a single element",
			input:    "root = *(DIGIT) 1*(ALPHA)\r\n",
			simplify: true,
			expected: "root=*DIGIT 1*ALPHA\r\n",
		},
		{
			name:     "simplify keeps needed parentheses",
			input:    "root = *(DIGIT / ALPHA) 2*(DIGIT ALPHA)\r\n",
			simplify: true,
			expected: "root=*(DIGIT/ALPHA) 2*(DIGIT ALPHA)\r\n",
		},
		{
			name:     "simplify nested groups",
			input:    "root = ((( \"a\" / \"b\" )))\r\n",
			simplify: true,
			expected: "root=\"a\"/\"b\"\r\n",
		},
		{
			name:     "simplify num-val series into char-val",
			input:    "root = %x4D.6F.6E ALPHA %x21\r\n",
			simplify: true,
			expected: "root=%s\"Mon\" ALPHA \"!\"\r\n",
		},
		{
			name:     "simplify joins adjacent num-vals",
			input:    "root = %x4D.6F.6E %x2C.20 %x21\r\n",
			simplify: true,
			expected: "root=%s\"Mon, !\"\r\n",
		},
		{
			name:     "simplify keeps shorter num-val",
			input:    "root = %x41 %d13.10\r\n",
			simplify: true,
			expected: "root=%x41 %d13.10\r\n",
		},
		{
			name:     "simplify single-valued range",
			input:    "root = %x41-41\r\n",
			simplify: true,
			expected: "root=%x41\r\n",
		},
		{
			name:     "simplify joins char-vals",
			input:    "root = \"a\" %s\"B\" \"1\"\r\n",
			simplify: true,
			expected: "root=\"a\" %s\"B1\"\r\n",
		},
		{
			name:     "simplify keeps distinct case sensitivity",
			input:    "root = \"a\" %s\"B\"\r\n",
			simplify: true,
			expected: "root=\"a\" %s\"B\"\r\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			output, err := Minify([]byte(testCase.input), &Options{Simplify: testCase.simplify})
			if err != nil {
				t.Fatalf("minify: %v", err)
			}

			if string(output) != testCase.expected {
				t.Fatalf("expected %q, got %q", testCase.expected, string(output))
			}
		})
	}
}

// TestMinifyListExtension checks that the "#" list operator survives
// minification, which is the point of reading it at all: its expansion is far
// longer than the operator. Expanding it is a separate request.
func TestMinifyListExtension(t *testing.T) {
	t.Parallel()

	const definition = "Accept = #( media-range [ weight ] )\r\n" +
		"media-range = token\r\n" +
		"weight = token\r\n" +
		"token = 1*( ALPHA / DIGIT )\r\n" +
		"OWS = *( SP / HTAB )\r\n"

	testCases := []struct {
		name     string
		options  *Options
		expected string
	}{
		{
			name:     "kept",
			options:  nil,
			expected: "Accept=#(media-range [weight])\r\n",
		},
		{
			name:    "expanded",
			options: &Options{ExpandLists: true},
			expected: "Accept=[(\",\"/(media-range [weight])) " +
				"*(OWS \",\" [OWS (media-range [weight])])]\r\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			output, err := Minify([]byte(definition), testCase.options)
			if err != nil {
				t.Fatalf("minify: %v", err)
			}

			accept, _, _ := strings.Cut(string(output), "\r\n")
			if accept+"\r\n" != testCase.expected {
				t.Fatalf("expected %q, got %q", testCase.expected, accept+"\r\n")
			}
		})
	}
}

// TestExpandingListsIsLonger checks the reason the operator is kept: writing
// it out makes the definition bigger, which is the opposite of the point.
func TestExpandingListsIsLonger(t *testing.T) {
	t.Parallel()

	const definition = "Accept = #( media-range [ weight ] )\r\n" +
		"media-range = token\r\n" +
		"weight = token\r\n" +
		"token = 1*ALPHA\r\n" +
		"OWS = *( SP / HTAB )\r\n"

	kept, err := Minify([]byte(definition), nil)
	if err != nil {
		t.Fatalf("minify: %v", err)
	}

	expanded, err := Minify([]byte(definition), &Options{ExpandLists: true})
	if err != nil {
		t.Fatalf("minify (expanded): %v", err)
	}

	if len(expanded) <= len(kept) {
		t.Fatalf("expected the expansion to be longer, got %d against %d", len(expanded), len(kept))
	}
}

func TestMinifyErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
	}{
		{name: "empty input"},
		{name: "not a grammar", input: "not a grammar"},
		{name: "missing dependency", input: "root = missing-rule\r\n"},
		{name: "duplicated rule", input: "root = \"a\"\r\nroot = \"b\"\r\n"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			output, err := Minify([]byte(testCase.input), nil)
			if err == nil {
				t.Fatalf("expected an error, got %q", string(output))
			}
			if !errors.Is(err, altshiftErrors.ErrParseError) {
				t.Fatalf("expected a parse error, got: %v", err)
			}
		})
	}
}

// TestMinifyPreservesGrammar checks that minification without simplification
// leaves the grammar itself untouched, by comparing the canonical
// representation of every rule before and after.
func TestMinifyPreservesGrammar(t *testing.T) {
	t.Parallel()

	for _, path := range grammarPaths(t) {
		t.Run(filepath.Base(filepath.Dir(path)), func(t *testing.T) {
			t.Parallel()

			input := readFile(t, path)

			grammar, err := abnf.ParseABNF(input)
			if err != nil {
				t.Fatalf("abnf parse abnf: %v", err)
			}

			output, err := Minify(input, nil)
			if err != nil {
				t.Fatalf("minify: %v", err)
			}

			minifiedGrammar, err := abnf.ParseABNF(output)
			if err != nil {
				t.Fatalf("abnf parse abnf (minified): %v", err)
			}

			if minifiedGrammar.String() != grammar.String() {
				t.Fatalf(
					"the minified grammar differs:\noriginal: %q\nminified: %q",
					grammar.String(),
					minifiedGrammar.String(),
				)
			}
		})
	}
}

// TestMinifyIsIdempotent checks that minifying an already minified grammar
// definition changes nothing, in both modes.
func TestMinifyIsIdempotent(t *testing.T) {
	t.Parallel()

	for _, simplify := range []bool{false, true} {
		for _, path := range grammarPaths(t) {
			name := filepath.Base(filepath.Dir(path))
			if simplify {
				name += " (simplified)"
			}

			t.Run(name, func(t *testing.T) {
				t.Parallel()

				options := &Options{Simplify: simplify}

				output, err := Minify(readFile(t, path), options)
				if err != nil {
					t.Fatalf("minify: %v", err)
				}

				repeated, err := Minify(output, options)
				if err != nil {
					t.Fatalf("minify (repeated): %v", err)
				}

				if string(repeated) != string(output) {
					t.Fatalf("expected %q, got %q", string(output), string(repeated))
				}
			})
		}
	}
}

// TestMinifyMatchesInput checks that a minified grammar matches the same
// inputs as the grammar it was made from, in both modes.
func TestMinifyMatchesInput(t *testing.T) {
	t.Parallel()

	const grammarText = "root = header *( \"; \" parameter )\r\n" +
		"header = ( token \"/\" token ) / %x2A\r\n" +
		"parameter = token \"=\" ( token / quoted )\r\n" +
		"token = 1*( ALPHA / DIGIT / \"-\" / \"+\" / \".\" )\r\n" +
		"quoted = DQUOTE *( %x20-21 / %x23-7E ) DQUOTE\r\n"

	inputs := []string{
		"text/plain",
		"*",
		"text/plain; charset=utf-8",
		"text/plain; charset=\"utf-8\"; boundary=x-1.2",
		"text/",
		"text/plain;charset=utf-8",
		"text/plain; =utf-8",
		"",
	}

	grammar, err := abnf.ParseABNF([]byte(grammarText))
	if err != nil {
		t.Fatalf("abnf parse abnf: %v", err)
	}

	for _, simplify := range []bool{false, true} {
		output, err := Minify([]byte(grammarText), &Options{Simplify: simplify})
		if err != nil {
			t.Fatalf("minify: %v", err)
		}

		minifiedGrammar, err := abnf.ParseABNF(output)
		if err != nil {
			t.Fatalf("abnf parse abnf (minified): %v", err)
		}

		for _, input := range inputs {
			paths, err := abnf.Parse([]byte(input), grammar, "root")
			if err != nil {
				t.Fatalf("abnf parse: %v", err)
			}

			minifiedPaths, err := abnf.Parse([]byte(input), minifiedGrammar, "root")
			if err != nil {
				t.Fatalf("abnf parse (minified): %v", err)
			}

			if matches, minifiedMatches := len(paths) != 0, len(minifiedPaths) != 0; matches != minifiedMatches {
				t.Fatalf(
					"input %q: expected matches=%t, got matches=%t (simplify=%t)",
					input,
					matches,
					minifiedMatches,
					simplify,
				)
			}
		}
	}
}

func TestNormalizeLineEndings(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "empty"},
		{name: "crlf kept", input: "a\r\nb\r\n", expected: "a\r\nb\r\n"},
		{name: "lf converted", input: "a\nb\n", expected: "a\r\nb\r\n"},
		{name: "mixed", input: "a\r\nb\n", expected: "a\r\nb\r\n"},
		{name: "unterminated", input: "a", expected: "a\r\n"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if normalized := string(normalizeLineEndings([]byte(testCase.input))); normalized != testCase.expected {
				t.Fatalf("expected %q, got %q", testCase.expected, normalized)
			}
		})
	}
}

// grammarPaths returns the paths of the ABNF grammar definitions of the
// module, which serve as real-world minification input.
func grammarPaths(t *testing.T) []string {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "pkg", "*", "*", "grammar.abnf"))
	if err != nil {
		t.Fatalf("filepath glob: %v", err)
	}

	nestedPaths, err := filepath.Glob(filepath.Join("..", "..", "..", "pkg", "*", "*", "*", "grammar.abnf"))
	if err != nil {
		t.Fatalf("filepath glob (nested): %v", err)
	}
	paths = append(paths, nestedPaths...)

	if len(paths) == 0 {
		t.Fatal("no grammar definitions were found")
	}

	return paths
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os read file: %v", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		t.Fatalf("empty grammar definition: %s", path)
	}

	return data
}
