package utils

import (
	"errors"
	"slices"
	"testing"

	"github.com/altshiftab/utils_go/pkg/abnf"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

const testGrammar = "root = key \"=\" value\r\nkey = 1*ALPHA\r\nvalue = 1*(ALPHA / DIGIT)\r\n"

func makeTestPath(t *testing.T, data []byte) *abnf.Path {
	t.Helper()

	grammar, err := abnf.ParseABNF([]byte(testGrammar))
	if err != nil {
		t.Fatalf("parse abnf: %v", err)
	}

	paths, err := GetParsedDataPaths(grammar, data, "root")
	if err != nil {
		t.Fatalf("get parsed data paths: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no paths")
	}

	return paths[0]
}

func TestGetParsedDataPaths(t *testing.T) {
	t.Parallel()

	grammar, err := abnf.ParseABNF([]byte(testGrammar))
	if err != nil {
		t.Fatalf("parse abnf: %v", err)
	}

	testCases := []struct {
		name             string
		grammar          *abnf.Grammar
		data             []byte
		rootRulename     string
		numPaths         int
		expectNilError   bool
		expectEmptyError bool
		expectedError    error
	}{
		{name: "valid data", grammar: grammar, data: []byte("a=1"), rootRulename: "root", numPaths: 1},
		{name: "non-root rulename", grammar: grammar, data: []byte("abc"), rootRulename: "key", numPaths: 1},
		{name: "non-matching data", grammar: grammar, data: []byte("=1"), rootRulename: "root", numPaths: 0},
		{name: "nil grammar", data: []byte("a=1"), rootRulename: "root", expectNilError: true},
		{
			name:          "empty data",
			grammar:       grammar,
			data:          nil,
			rootRulename:  "root",
			expectedError: altshiftErrors.ErrSyntaxError,
		},
		{
			name:             "empty root rulename",
			grammar:          grammar,
			data:             []byte("a=1"),
			expectEmptyError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			paths, err := GetParsedDataPaths(testCase.grammar, testCase.data, testCase.rootRulename)
			if testCase.expectNilError {
				if err == nil {
					t.Fatal("expected an error")
				}
				if _, ok := errors.AsType[*nil_error.Error](err); !ok {
					t.Fatalf("expected a nil error, got: %v", err)
				}
				return
			}
			if testCase.expectEmptyError {
				if err == nil {
					t.Fatal("expected an error")
				}
				if _, ok := errors.AsType[*empty_error.Error](err); !ok {
					t.Fatalf("expected an empty error, got: %v", err)
				}
				return
			}
			if expectedError := testCase.expectedError; expectedError != nil {
				if err == nil {
					t.Fatal("expected an error")
				}
				if !errors.Is(err, expectedError) {
					t.Fatalf("expected %v, got: %v", expectedError, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("get parsed data paths: %v", err)
			}
			if len(paths) != testCase.numPaths {
				t.Fatalf("expected %d paths, got %d", testCase.numPaths, len(paths))
			}
		})
	}
}

func TestSearchPath(t *testing.T) {
	t.Parallel()

	data := []byte("abc=1")
	path := makeTestPath(t, data)

	testCases := []struct {
		name          string
		names         []string
		maxDepth      int
		searchMatch   bool
		expectedRules []string
	}{
		{name: "unlimited depth", names: []string{"key", "value"}, maxDepth: -1, expectedRules: []string{"key", "value"}},
		{
			name:          "finite max depth does not limit traversal",
			names:         []string{"key", "value"},
			maxDepth:      1,
			expectedRules: []string{"key", "value"},
		},
		{
			name:          "match subtrees are pruned without search match",
			names:         []string{"root", "key"},
			maxDepth:      -1,
			expectedRules: []string{"root"},
		},
		{
			name:          "match subtrees are searched with search match",
			names:         []string{"root", "key"},
			maxDepth:      -1,
			searchMatch:   true,
			expectedRules: []string{"root", "key"},
		},
		{name: "absent name", names: []string{"missing"}, maxDepth: -1},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			paths := SearchPath(path, testCase.names, testCase.maxDepth, testCase.searchMatch)

			var rules []string
			for _, matchedPath := range paths {
				rules = append(rules, matchedPath.MatchRule)
			}

			if !slices.Equal(rules, testCase.expectedRules) {
				t.Fatalf("expected rules %v, got %v", testCase.expectedRules, rules)
			}
		})
	}
}

func TestSearchPathSingle(t *testing.T) {
	t.Parallel()

	data := []byte("abc=1")
	path := makeTestPath(t, data)

	testCases := []struct {
		name          string
		names         []string
		expectedValue string
		expectNil     bool
	}{
		{name: "present name", names: []string{"key"}, expectedValue: "abc"},
		{name: "multiple names", names: []string{"key", "value"}, expectedValue: "abc"},
		{name: "absent name", names: []string{"missing"}, expectNil: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			matchedPath := SearchPathSingle(path, testCase.names, -1, false)
			if testCase.expectNil {
				if matchedPath != nil {
					t.Fatalf("expected nil, got %v", matchedPath)
				}
				return
			}
			if matchedPath == nil {
				t.Fatal("nil path")
			}
			if value := string(ExtractPathValue(data, matchedPath)); value != testCase.expectedValue {
				t.Fatalf("expected value %q, got %q", testCase.expectedValue, value)
			}
		})
	}
}

func TestSearchPathSingleName(t *testing.T) {
	t.Parallel()

	data := []byte("abc=1")
	path := makeTestPath(t, data)

	valuePath := SearchPathSingleName(path, "value", -1, false)
	if valuePath == nil {
		t.Fatal("nil path")
	}
	if value := string(ExtractPathValue(data, valuePath)); value != "1" {
		t.Fatalf("expected value %q, got %q", "1", value)
	}
}
