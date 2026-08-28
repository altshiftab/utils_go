package argument_parser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	argumentParserErrors "github.com/altshiftab/utils_go/pkg/cli/argument_parser/errors"
	"github.com/altshiftab/utils_go/pkg/cli/argument_parser/option"
	"github.com/altshiftab/utils_go/pkg/testing/cmp"
)

var diffOpts = []cmp.Option{cmp.EquateEmpty()}

var errSubparser = errors.New("subparser failure")

// stubSubparser records the arguments handed to it by a parent parser.
type stubSubparser struct {
	command   string
	arguments []string
	err       error
}

func (subparser *stubSubparser) GetCommand() string {
	return subparser.command
}

func (subparser *stubSubparser) ParseArgs(arguments []string) error {
	subparser.arguments = arguments
	return subparser.err
}

func TestArgumentParserParseArgs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		options      []option.Option
		args         []string
		intValue     int
		intsValue    []int
		strValue     string
		stringsValue []string
		boolValue    bool
		countValue   int
		wantErr      error
	}{
		{
			name: "full",
			options: []option.Option{
				option.NewIntOption('i', "int", "An int option", false, nil),
				option.NewStringOption('s', "str", "A string option", false, nil),
				option.NewBoolOption('b', "bool", "A bool option", false, nil),
				option.NewStringsOption('a', "array", "An array option", false, nil),
				option.NewIntsOption('n', "numbers", "An array of ints option", false, nil),
			},
			args:         []string{"-i", "42", "--str", "abc", "--bool", "-a", "a", "b", "-n", "1", "2"},
			intValue:     42,
			intsValue:    []int{1, 2},
			strValue:     "abc",
			stringsValue: []string{"a", "b"},
			boolValue:    true,
			wantErr:      nil,
		},
		{
			name: "multiple same",
			options: []option.Option{
				option.NewIntOption('i', "int", "An int option", false, nil),
			},
			args:     []string{"-i", "1", "-i", "2"},
			intValue: 2,
		},
		{
			name: "counted option repeated",
			options: []option.Option{
				option.NewCountedOption('v', "verbose", "Verbosity", false, nil),
			},
			args:       []string{"-v", "-v", "-v"},
			countValue: 3,
		},
		{
			name: "counted option clustered",
			options: []option.Option{
				option.NewCountedOption('v', "verbose", "Verbosity", false, nil),
			},
			args:       []string{"-vvv"},
			countValue: 3,
		},
		{
			name: "clustered zero-arg options",
			options: []option.Option{
				option.NewBoolOption('b', "bool", "A bool option", false, nil),
				option.NewCountedOption('v', "verbose", "Verbosity", false, nil),
			},
			args:       []string{"-bv"},
			boolValue:  true,
			countValue: 1,
		},
		{
			name: "unset option 1",
			options: []option.Option{
				option.NewIntOption('i', "int", "An int option", false, nil),
			},
			args:    []string{"-i"},
			wantErr: argumentParserErrors.ErrUnsetOption,
		},
		{
			name: "unset option 2",
			options: []option.Option{
				option.NewIntOption('i', "int", "An int option", false, nil),
				option.NewBoolOption('b', "bool", "A bool option", false, nil),
			},
			args:    []string{"-i", "42", "--bool", "--int"},
			wantErr: argumentParserErrors.ErrUnsetOption,
		},
		{
			name: "name not found",
			options: []option.Option{
				option.NewIntOption('i', "int", "An int option", false, nil),
			},
			args:    []string{"--unknown", "1"},
			wantErr: argumentParserErrors.ErrNameNotFound,
		},
		{
			name: "no current option",
			options: []option.Option{
				option.NewIntOption('i', "int", "An int option", false, nil),
			},
			args:    []string{"positional"},
			wantErr: argumentParserErrors.ErrUnexpectedArgument,
		},
		{
			name: "a terminator with nothing after it",
			options: []option.Option{
				option.NewIntOption('i', "int", "An int option", false, nil),
			},
			args:     []string{"-i", "42", "--"},
			intValue: 42,
		},
		{
			// Without a Rest to collect them, arguments after the terminator have nowhere to go and
			// must not be dropped on the floor.
			name: "arguments after the terminator need somewhere to go",
			options: []option.Option{
				option.NewIntOption('i', "int", "An int option", false, nil),
			},
			args:    []string{"-i", "42", "--", "-i", "99"},
			wantErr: argumentParserErrors.ErrUnexpectedArgument,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var intValue int
			var stringValue string
			var boolValue bool
			var countValue int
			stringsValue := make([]string, 0)
			intsValue := make([]int, 0)

			for _, opt := range testCase.options {
				switch typedOpt := opt.(type) {
				case *option.IntOption:
					typedOpt.Value = &intValue
				case *option.StringOption:
					typedOpt.Value = &stringValue
				case *option.BoolOption:
					typedOpt.Value = &boolValue
				case *option.StringsOption:
					typedOpt.Value = &stringsValue
				case *option.IntsOption:
					typedOpt.Value = &intsValue
				case *option.CountedOption:
					typedOpt.Count = &countValue
				default:
					t.Fatalf("Unexpected option type: %T", opt)
					return
				}
			}

			parser := &Parser{Options: testCase.options}

			err := parser.ParseArgs(testCase.args)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("unexpected error = %v, want %v", err, testCase.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error = %v", err)
				return
			}

			if intValue != testCase.intValue {
				t.Errorf("Expected int value = %v, got %v", testCase.intValue, intValue)
			}

			if stringValue != testCase.strValue {
				t.Errorf("Expected string value = %v, got %v", testCase.strValue, stringValue)
			}

			if boolValue != testCase.boolValue {
				t.Errorf("Expected bool value = %v, got %v", testCase.boolValue, boolValue)
			}

			if countValue != testCase.countValue {
				t.Errorf("Expected count value = %v, got %v", testCase.countValue, countValue)
			}

			if diff := cmp.Diff(testCase.stringsValue, stringsValue, diffOpts...); diff != "" {
				t.Errorf("strings value mismatch (-expected +got):\n%s", diff)
			}

			if diff := cmp.Diff(testCase.intsValue, intsValue, diffOpts...); diff != "" {
				t.Errorf("ints value mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}

func TestParseArgsSubparser(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		args          []string
		subparserErr  error
		wantArguments []string
		wantDispatch  bool
		wantErr       error
	}{
		{
			name:          "dispatches to matching command",
			args:          []string{"serve", "--port", "8080"},
			wantArguments: []string{"--port", "8080"},
			wantDispatch:  true,
		},
		{
			name:          "dispatches with no remaining arguments",
			args:          []string{"serve"},
			wantArguments: []string{},
			wantDispatch:  true,
		},
		{
			name:          "propagates subparser error",
			args:          []string{"serve", "-x"},
			subparserErr:  errSubparser,
			wantArguments: []string{"-x"},
			wantDispatch:  true,
			wantErr:       errSubparser,
		},
		{
			name:         "falls through to options when command does not match",
			args:         []string{"-i", "7"},
			wantDispatch: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			subparser := &stubSubparser{command: "serve", err: testCase.subparserErr}

			var intValue int
			parser := &Parser{
				Options: []option.Option{option.NewIntOption('i', "int", "An int option", false, &intValue)},
				Parsers: []Subparser{nil, subparser},
			}

			err := parser.ParseArgs(testCase.args)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("unexpected error = %v, want %v", err, testCase.wantErr)
				}
			} else if err != nil {
				t.Errorf("unexpected error = %v", err)
			}

			if testCase.wantDispatch {
				if diff := cmp.Diff(testCase.wantArguments, subparser.arguments, diffOpts...); diff != "" {
					t.Errorf("subcommand arguments mismatch (-expected +got):\n%s", diff)
				}
				return
			}

			if subparser.arguments != nil {
				t.Errorf("expected no dispatch, got arguments %v", subparser.arguments)
			}

			if intValue != 7 {
				t.Errorf("Expected int value = 7, got %v", intValue)
			}
		})
	}
}

func TestFormatHelp(t *testing.T) {
	t.Parallel()

	parser := &Parser{
		ProgramName: "myapp",
		Width:       80,
		Description: "A test application.",
		Options: []option.Option{
			option.NewIntOption('i', "int", "An int option", false, nil),
			option.NewStringOption('s', "str", "A string option", false, nil),
			option.NewBoolOption('b', "bool", "A bool option", false, nil),
			option.NewStringsOption('a', "array", "An array option", false, nil),
			option.NewIntsOption('n', "numbers", "An array of ints option", false, nil),
		},
	}

	got := parser.FormatHelp()

	// The array option's term reaches into the description column, so its description moves to the
	// following line, as argparse does at its own max_help_position.
	expected := "Usage: myapp [-h] [-i INT] [-s STRING] [-b] [-a STRING...] [-n INT...]\n" +
		"\n" +
		"A test application.\n" +
		"\n" +
		"Options:\n" +
		"  -i, --int INT         An int option\n" +
		"  -s, --str STRING      A string option\n" +
		"  -b, --bool            A bool option\n" +
		"  -a, --array STRING...\n" +
		"                        An array option\n" +
		"  -n, --numbers INT...  An array of ints option\n" +
		"  -h, --help            Show this help message and exit\n"

	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("FormatHelp mismatch (-expected +got):\n%s", diff)
	}
}

func TestFormatHelpLongNameOnly(t *testing.T) {
	t.Parallel()

	parser := &Parser{
		ProgramName: "myapp",
		Width:       80,
		Options: []option.Option{
			nil,
			option.NewStringOption(0, "only-long", "A long-only option", false, nil),
		},
	}

	got := parser.FormatHelp()

	expected := "Usage: myapp [-h] [--only-long STRING]\n" +
		"\n" +
		"Options:\n" +
		"      --only-long STRING\n" +
		"                        A long-only option\n" +
		"  -h, --help            Show this help message and exit\n"

	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("FormatHelp mismatch (-expected +got):\n%s", diff)
	}
}

func TestFormatHelpCommands(t *testing.T) {
	t.Parallel()

	parser := &Parser{
		ProgramName: "myapp",
		Width:       80,
		Options:     []option.Option{option.NewBoolOption('v', "verbose", "Verbose output", false, nil)},
		Parsers: []Subparser{
			&stubSubparser{command: "serve"},
			nil,
			&stubSubparser{command: "migrate"},
		},
	}

	got := parser.FormatHelp()

	expected := "Usage: myapp [-h] [-v] {serve,migrate} ...\n" +
		"\n" +
		"Commands:\n" +
		"  serve\n" +
		"  migrate\n" +
		"\n" +
		"Options:\n" +
		"  -v, --verbose  Verbose output\n" +
		"  -h, --help     Show this help message and exit\n"

	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("FormatHelp mismatch (-expected +got):\n%s", diff)
	}
}

func TestFormatHelpRequiredOptionIsUnbracketed(t *testing.T) {
	t.Parallel()

	parser := &Parser{
		ProgramName: "myapp",
		Width:       80,
		Options: []option.Option{
			option.NewStringOption('s', "str", "Required", true, nil),
			option.NewIntOption('i', "int", "Optional", false, nil),
		},
	}

	expected := "Usage: myapp [-h] -s STRING [-i INT]"
	if got, _, _ := strings.Cut(parser.FormatHelp(), "\n"); got != expected {
		t.Errorf("usage line = %q, want %q", got, expected)
	}
}

func TestGetProgramNameIsABaseName(t *testing.T) {
	t.Parallel()

	// os.Args[0] is a full path to the test binary; help must not print it verbatim.
	parser := &Parser{Options: []option.Option{option.NewBoolOption('v', "verbose", "V", false, nil)}}

	got := parser.getProgramName()
	if strings.Contains(got, string(os.PathSeparator)) {
		t.Errorf("getProgramName() = %q, want a base name without a path separator", got)
	}
	if got != filepath.Base(os.Args[0]) {
		t.Errorf("getProgramName() = %q, want %q", got, filepath.Base(os.Args[0]))
	}
}

func TestParseArgsHelp(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"-h", "--help"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var output strings.Builder
			parser := &Parser{
				ProgramName: "myapp",
				Output:      &output,
				Options: []option.Option{
					option.NewBoolOption('v', "verbose", "Verbose output", false, nil),
				},
			}

			err := parser.ParseArgs([]string{name})
			if !errors.Is(err, argumentParserErrors.ErrHelp) {
				t.Errorf("ParseArgs(%q) error = %v, want ErrHelp", name, err)
			}

			if diff := cmp.Diff(parser.FormatHelp(), output.String()); diff != "" {
				t.Errorf("help output mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}

func TestParseArgsRequired(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		args    []string
		wantErr error
	}{
		{name: "no arguments", args: nil, wantErr: argumentParserErrors.ErrMissingRequiredOption},
		{name: "other option only", args: []string{"-i", "1"}, wantErr: argumentParserErrors.ErrMissingRequiredOption},
		{name: "terminated early", args: []string{"--"}, wantErr: argumentParserErrors.ErrMissingRequiredOption},
		{name: "given by short name", args: []string{"-s", "x"}},
		{name: "given by long name", args: []string{"--str", "x"}},
		{name: "given inline", args: []string{"--str=x"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var stringValue string
			var intValue int
			parser := &Parser{
				ProgramName: "myapp",
				Options: []option.Option{
					option.NewStringOption('s', "str", "Required", true, &stringValue),
					option.NewIntOption('i', "int", "Optional", false, &intValue),
				},
			}

			err := parser.ParseArgs(testCase.args)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("unexpected error = %v, want %v", err, testCase.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error = %v", err)
			}
		})
	}
}

func TestParseArgsInlineValue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		args        []string
		wantInt     int
		wantStrings []string
		wantBool    bool
		wantErr     error
	}{
		{name: "int", args: []string{"--int=42"}, wantInt: 42},
		{name: "negative int", args: []string{"--int=-3"}, wantInt: -3},
		{name: "strings accumulate", args: []string{"--array=a", "--array=b"}, wantStrings: []string{"a", "b"}},
		{
			name:        "inline value does not consume the next argument",
			args:        []string{"--array=a", "--int", "1"},
			wantInt:     1,
			wantStrings: []string{"a"},
		},
		{
			name:    "a trailing argument after an inline value is not the option's",
			args:    []string{"--array=a", "b"},
			wantErr: argumentParserErrors.ErrUnexpectedArgument,
		},
		{
			name:    "zero-nargs option rejects an inline value",
			args:    []string{"--bool=false"},
			wantErr: argumentParserErrors.ErrUnexpectedOptionValue,
		},
		{
			name:    "unknown name with an inline value",
			args:    []string{"--nope=1"},
			wantErr: argumentParserErrors.ErrNameNotFound,
		},
		{name: "empty inline value reaches the option", args: []string{"--array="}, wantStrings: []string{""}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var intValue int
			var boolValue bool
			stringsValue := make([]string, 0)
			parser := &Parser{
				ProgramName: "myapp",
				Options: []option.Option{
					option.NewIntOption('i', "int", "An int option", false, &intValue),
					option.NewStringsOption('a', "array", "An array option", false, &stringsValue),
					option.NewBoolOption('b', "bool", "A bool option", false, &boolValue),
				},
			}

			err := parser.ParseArgs(testCase.args)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("unexpected error = %v, want %v", err, testCase.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error = %v", err)
				return
			}

			if intValue != testCase.wantInt {
				t.Errorf("int = %d, want %d", intValue, testCase.wantInt)
			}
			if boolValue != testCase.wantBool {
				t.Errorf("bool = %v, want %v", boolValue, testCase.wantBool)
			}
			if diff := cmp.Diff(testCase.wantStrings, stringsValue, diffOpts...); diff != "" {
				t.Errorf("strings mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}

func TestParseArgsNegativeNumberValues(t *testing.T) {
	t.Parallel()

	t.Run("negative numbers are values, not options", func(t *testing.T) {
		t.Parallel()

		var intValue int
		intsValue := make([]int, 0)
		parser := &Parser{
			ProgramName: "myapp",
			Options: []option.Option{
				option.NewIntOption('i', "int", "An int option", false, &intValue),
				option.NewIntsOption('n', "numbers", "Ints", false, &intsValue),
			},
		}

		if err := parser.ParseArgs([]string{"-i", "-3", "-n", "-1", "-2"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if intValue != -3 {
			t.Errorf("int = %d, want -3", intValue)
		}
		if diff := cmp.Diff([]int{-1, -2}, intsValue, diffOpts...); diff != "" {
			t.Errorf("ints mismatch (-expected +got):\n%s", diff)
		}
	})

	t.Run("an option named like a number wins", func(t *testing.T) {
		t.Parallel()

		var count int
		parser := &Parser{
			ProgramName: "myapp",
			Options:     []option.Option{option.NewCountedOption('1', "one", "Counter", false, &count)},
		}

		if err := parser.ParseArgs([]string{"-1"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 1 {
			t.Errorf("count = %d, want 1", count)
		}
	})

	t.Run("a negative number with no pending option is still positional", func(t *testing.T) {
		t.Parallel()

		var intValue int
		parser := &Parser{
			ProgramName: "myapp",
			Options:     []option.Option{option.NewIntOption('i', "int", "An int option", false, &intValue)},
		}

		err := parser.ParseArgs([]string{"-3"})
		if !errors.Is(err, argumentParserErrors.ErrUnexpectedArgument) {
			t.Errorf("error = %v, want ErrUnexpectedArgument", err)
		}
	})
}

func TestParseArgsLongOnlyOptionsDoNotCollide(t *testing.T) {
	t.Parallel()

	var alpha, beta string
	parser := &Parser{
		ProgramName: "myapp",
		Options: []option.Option{
			option.NewStringOption(0, "alpha", "Alpha", false, &alpha),
			option.NewStringOption(0, "beta", "Beta", false, &beta),
		},
	}

	if err := parser.ParseArgs([]string{"--alpha", "1", "--beta", "2"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alpha != "1" || beta != "2" {
		t.Errorf("alpha = %q, beta = %q, want %q and %q", alpha, beta, "1", "2")
	}
}

func TestParseArgument(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		in   string
		want *parsedArgument
	}{
		{name: "empty", in: "", want: nil},
		{name: "short name", in: "-n", want: &parsedArgument{names: []string{"n"}}},
		{name: "long name", in: "--filename", want: &parsedArgument{names: []string{"filename"}, long: true}},
		{name: "clustered short names", in: "-yes", want: &parsedArgument{names: []string{"y", "e", "s"}}},
		{name: "bare word", in: "no", want: nil},
		{name: "embedded double dash", in: "nope--nope", want: nil},
		{name: "embedded dash", in: "nope-nope", want: nil},
		{
			name: "long name with inline value",
			in:   "--int=42",
			want: &parsedArgument{names: []string{"int"}, inlineValue: new("42"), long: true},
		},
		{
			name: "long name with empty inline value",
			in:   "--str=",
			want: &parsedArgument{names: []string{"str"}, inlineValue: new(""), long: true},
		},
		{
			name: "inline value containing an equals sign",
			in:   "--expr=a=b",
			want: &parsedArgument{names: []string{"expr"}, inlineValue: new("a=b"), long: true},
		},
		{name: "inline value without a name", in: "--=42", want: nil},
		{
			name: "short names are not split on equals",
			in:   "-i=42",
			want: &parsedArgument{names: []string{"i", "=", "4", "2"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := parseArgument(testCase.in)

			if testCase.want == nil {
				if got != nil {
					t.Fatalf("expected no parsed argument, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected a parsed argument, got nil")
			}

			if diff := cmp.Diff(testCase.want.names, got.names, diffOpts...); diff != "" {
				t.Errorf("names mismatch (-expected +got):\n%s", diff)
			}
			if diff := cmp.Diff(testCase.want.inlineValue, got.inlineValue); diff != "" {
				t.Errorf("inline value mismatch (-expected +got):\n%s", diff)
			}
			if got.long != testCase.want.long {
				t.Errorf("long = %v, want %v", got.long, testCase.want.long)
			}
		})
	}
}

func TestLongAndShortNamesAreSeparateNamespaces(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		args      []string
		wantShort string
		wantLong  int
		wantErr   error
	}{
		{name: "short name with one dash", args: []string{"-a", "x"}, wantShort: "x"},
		{name: "long name with two dashes", args: []string{"--long", "5"}, wantLong: 5},
		{
			name:    "a short name is not reachable with two dashes",
			args:    []string{"--a", "x"},
			wantErr: argumentParserErrors.ErrNameNotFound,
		},
		{
			name:    "a long name is not reachable with one dash",
			args:    []string{"-long", "5"},
			wantErr: argumentParserErrors.ErrNameNotFound,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var shortValue string
			var longValue int
			parser := &Parser{
				ProgramName: "myapp",
				Options: []option.Option{
					option.NewStringOption('a', "", "A short-only option", false, &shortValue),
					option.NewIntOption(0, "long", "A long-only option", false, &longValue),
				},
			}

			err := parser.ParseArgs(testCase.args)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("unexpected error = %v, want %v", err, testCase.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error = %v", err)
			}
			if shortValue != testCase.wantShort {
				t.Errorf("short = %q, want %q", shortValue, testCase.wantShort)
			}
			if longValue != testCase.wantLong {
				t.Errorf("long = %d, want %d", longValue, testCase.wantLong)
			}
		})
	}
}

func TestShortNameMayEqualLongName(t *testing.T) {
	t.Parallel()

	var value int
	parser := &Parser{
		ProgramName: "myapp",
		Options:     []option.Option{option.NewIntOption('n', "n", "An int option", false, &value)},
	}

	if err := parser.ParseArgs([]string{"-n", "5"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != 5 {
		t.Errorf("value = %d, want 5", value)
	}

	if err := parser.ParseArgs([]string{"--n", "7"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != 7 {
		t.Errorf("value = %d, want 7", value)
	}
}

func TestIsNegativeNumber(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		in   string
		want bool
	}{
		{name: "integer", in: "-3", want: true},
		{name: "multiple digits", in: "-4711", want: true},
		{name: "decimal", in: "-3.5", want: true},
		{name: "decimal without an integer part", in: "-.5", want: true},
		{name: "bare dash", in: "-", want: false},
		{name: "trailing decimal point", in: "-3.", want: false},
		{name: "two decimal points", in: "-3.5.6", want: false},
		{name: "positive", in: "3", want: false},
		{name: "exponent", in: "-1e5", want: false},
		{name: "infinity", in: "-inf", want: false},
		{name: "option", in: "-i", want: false},
		{name: "long option", in: "--int", want: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := isNegativeNumber(testCase.in); got != testCase.want {
				t.Errorf("isNegativeNumber(%q) = %v, want %v", testCase.in, got, testCase.want)
			}
		})
	}
}

func TestMakeNameTables(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		options       []option.Option
		wantErr       error
		containsShort map[string]bool
		containsLong  map[string]bool
	}{
		{
			name:    "No options",
			options: nil,
			wantErr: nil,
		},
		{
			name: "Duplicate long name",
			options: []option.Option{
				option.NewIntOption('a', "same", "usage", false, nil),
				option.NewIntOption('b', "same", "usage2", false, nil),
			},
			wantErr: argumentParserErrors.ErrMultipleOptionsWithSameName,
		},
		{
			name: "Duplicate short name",
			options: []option.Option{
				option.NewIntOption('a', "as", "usage", false, nil),
				option.NewIntOption('a', "bs", "usage2", false, nil),
			},
			wantErr: argumentParserErrors.ErrMultipleOptionsWithSameName,
		},
		{
			name: "OK names",
			options: []option.Option{
				option.NewIntOption('a', "as", "usage", false, nil),
				option.NewIntOption('b', "bs", "usage2", false, nil),
			},
			wantErr:       nil,
			containsShort: map[string]bool{"a": true, "b": true, "as": false},
			containsLong:  map[string]bool{"as": true, "bs": true, "a": false},
		},
		{
			// A short name and a long name are separate namespaces, so the same text may serve as
			// both without colliding.
			name:          "Short name equal to long name",
			options:       []option.Option{option.NewIntOption('n', "n", "usage", false, nil)},
			wantErr:       nil,
			containsShort: map[string]bool{"n": true},
			containsLong:  map[string]bool{"n": true},
		},
		{
			name:          "Nil options are skipped",
			options:       []option.Option{nil, option.NewIntOption('a', "as", "usage", false, nil)},
			wantErr:       nil,
			containsShort: map[string]bool{"a": true, "b": false},
			containsLong:  map[string]bool{"as": true},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := makeNameTables(testCase.options)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("Expected error %v, got %v", testCase.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			for name, wantPresent := range testCase.containsShort {
				if _, found := got.lookup(name, false); found != wantPresent {
					t.Errorf("Short name %q present: %v, want %v", name, found, wantPresent)
				}
			}
			for name, wantPresent := range testCase.containsLong {
				if _, found := got.lookup(name, true); found != wantPresent {
					t.Errorf("Long name %q present: %v, want %v", name, found, wantPresent)
				}
			}
		})
	}
}

func TestParseArgsClearsSatisfiedOption(t *testing.T) {
	t.Parallel()

	// A value-taking option must stop consuming once it has what it takes; otherwise a later bare
	// argument silently overwrites it.
	testCases := []struct {
		name        string
		args        []string
		wantInt     int
		wantStrings []string
		wantRest    []string
	}{
		{
			name:     "a bare argument after a flag is not the earlier option's",
			args:     []string{"-i", "42", "-b", "99"},
			wantInt:  42,
			wantRest: []string{"99"},
		},
		{
			name:     "a bare argument after a satisfied option is not its second value",
			args:     []string{"-i", "42", "99"},
			wantInt:  42,
			wantRest: []string{"99"},
		},
		{
			name:        "an accumulating option keeps consuming",
			args:        []string{"-a", "x", "y", "z"},
			wantStrings: []string{"x", "y", "z"},
		},
		{
			name:        "an accumulating option stops at the next option",
			args:        []string{"-a", "x", "-i", "1", "y"},
			wantInt:     1,
			wantStrings: []string{"x"},
			wantRest:    []string{"y"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var intValue int
			var boolValue bool
			var rest []string
			stringsValue := make([]string, 0)

			parser := &Parser{
				ProgramName: "myapp",
				Rest:        &rest,
				Options: []option.Option{
					option.NewIntOption('i', "int", "An int option", false, &intValue),
					option.NewBoolOption('b', "bool", "A bool option", false, &boolValue),
					option.NewStringsOption('a', "array", "An array option", false, &stringsValue),
				},
			}

			if err := parser.ParseArgs(testCase.args); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if intValue != testCase.wantInt {
				t.Errorf("int = %d, want %d", intValue, testCase.wantInt)
			}
			if diff := cmp.Diff(testCase.wantStrings, stringsValue, diffOpts...); diff != "" {
				t.Errorf("strings mismatch (-expected +got):\n%s", diff)
			}
			if diff := cmp.Diff(testCase.wantRest, rest, diffOpts...); diff != "" {
				t.Errorf("rest mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}

func TestParseArgsRest(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		args     []string
		wantInt  int
		wantRest []string
		wantErr  error
	}{
		{name: "nothing left over", args: []string{"-i", "1"}, wantInt: 1},
		{name: "positionals", args: []string{"a", "-i", "1", "b"}, wantInt: 1, wantRest: []string{"a", "b"}},
		{
			name:     "everything after the terminator is kept verbatim",
			args:     []string{"-i", "1", "--", "-b", "--int", "x"},
			wantInt:  1,
			wantRest: []string{"-b", "--int", "x"},
		},
		{
			name:    "the terminator does not excuse an option left without a value",
			args:    []string{"-i", "--"},
			wantErr: argumentParserErrors.ErrUnsetOption,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var intValue int
			var boolValue bool
			var rest []string

			parser := &Parser{
				ProgramName: "myapp",
				Rest:        &rest,
				Options: []option.Option{
					option.NewIntOption('i', "int", "An int option", false, &intValue),
					option.NewBoolOption('b', "bool", "A bool option", false, &boolValue),
				},
			}

			err := parser.ParseArgs(testCase.args)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("unexpected error = %v, want %v", err, testCase.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if intValue != testCase.wantInt {
				t.Errorf("int = %d, want %d", intValue, testCase.wantInt)
			}
			if diff := cmp.Diff(testCase.wantRest, rest, diffOpts...); diff != "" {
				t.Errorf("rest mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}

func TestParseArgsWithoutRestRejectsPositionals(t *testing.T) {
	t.Parallel()

	parser := &Parser{
		ProgramName: "myapp",
		Options:     []option.Option{option.NewIntOption('i', "int", "An int option", false, nil)},
	}

	if err := parser.ParseArgs([]string{"leftover"}); !errors.Is(err, argumentParserErrors.ErrUnexpectedArgument) {
		t.Errorf("error = %v, want ErrUnexpectedArgument", err)
	}
}

func TestHelpOptionIsYieldedToAnOptionOfTheSameName(t *testing.T) {
	t.Parallel()

	t.Run("a claimed -h reaches the option", func(t *testing.T) {
		t.Parallel()

		var host string
		var output strings.Builder
		parser := &Parser{
			ProgramName: "myapp",
			Output:      &output,
			Options:     []option.Option{option.NewStringOption('h', "host", "Host", false, &host)},
		}

		if err := parser.ParseArgs([]string{"-h", "example.com"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if host != "example.com" {
			t.Errorf("host = %q, want %q", host, "example.com")
		}
		if output.Len() != 0 {
			t.Errorf("expected no help output, got %q", output.String())
		}

		// --help is unclaimed, so it still works, and help lists only the name it may use.
		if err := parser.ParseArgs([]string{"--help"}); !errors.Is(err, argumentParserErrors.ErrHelp) {
			t.Errorf("error = %v, want ErrHelp", err)
		}
		if strings.Contains(parser.FormatHelp(), "-h, --help") {
			t.Errorf("help must not offer a claimed -h:\n%s", parser.FormatHelp())
		}
	})

	t.Run("DisableHelp withholds both names", func(t *testing.T) {
		t.Parallel()

		parser := &Parser{
			ProgramName: "myapp",
			DisableHelp: true,
			Options:     []option.Option{option.NewBoolOption('v', "verbose", "Verbose", false, nil)},
		}

		if err := parser.ParseArgs([]string{"-h"}); !errors.Is(err, argumentParserErrors.ErrNameNotFound) {
			t.Errorf("error = %v, want ErrNameNotFound", err)
		}
		if strings.Contains(parser.FormatHelp(), "--help") {
			t.Errorf("help must not be offered when disabled:\n%s", parser.FormatHelp())
		}
	})
}

func TestParserIsASubparser(t *testing.T) {
	t.Parallel()

	var port int
	serve := &Parser{
		Command:     "serve",
		Description: "Serve the thing.",
		Options:     []option.Option{option.NewIntOption('p', "port", "Port", false, &port)},
	}

	var verbose bool
	parser := &Parser{
		ProgramName: "myapp",
		Width:       80,
		Options:     []option.Option{option.NewBoolOption('v', "verbose", "Verbose", false, &verbose)},
		Parsers:     []Subparser{serve},
	}

	if err := parser.ParseArgs([]string{"serve", "--port", "8080"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port != 8080 {
		t.Errorf("port = %d, want 8080", port)
	}

	if got := parser.FormatHelp(); !strings.Contains(got, "  serve") || !strings.Contains(got, "Serve the thing.") {
		t.Errorf("a subparser's description belongs in the parent's help:\n%s", got)
	}
}

func TestWrapText(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		text  string
		width int
		want  []string
	}{
		{name: "empty", text: "", width: 10, want: nil},
		{name: "whitespace only", text: "   \t ", width: 10, want: nil},
		{name: "fits", text: "one two", width: 10, want: []string{"one two"}},
		{name: "wraps", text: "one two three four", width: 9, want: []string{"one two", "three", "four"}},
		{name: "collapses whitespace", text: "one   two", width: 10, want: []string{"one two"}},
		{
			name:  "a word longer than the width is not broken",
			text:  "supercalifragilistic x",
			width: 5,
			want:  []string{"supercalifragilistic", "x"},
		},
		{name: "non-positive width", text: "one two", width: 0, want: []string{"one two"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := wrapText(testCase.text, testCase.width)
			if diff := cmp.Diff(testCase.want, got, diffOpts...); diff != "" {
				t.Errorf("mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}

func TestFormatHelpWraps(t *testing.T) {
	t.Parallel()

	parser := &Parser{
		ProgramName: "myapp",
		Width:       60,
		Description: "A description long enough that it has to be broken across more than one line.",
		Options: []option.Option{
			option.NewStringOption('m', "mode", "Selects the operating mode, described at length so that it wraps.", false, nil),
		},
	}

	got := parser.FormatHelp()

	for line := range strings.SplitSeq(strings.TrimRight(got, "\n"), "\n") {
		if len(line) > 60 {
			t.Errorf("line of %d columns exceeds the width:\n%q", len(line), line)
		}
	}

	expected := "Usage: myapp [-h] [-m STRING]\n" +
		"\n" +
		"A description long enough that it has to be broken across\n" +
		"more than one line.\n" +
		"\n" +
		"Options:\n" +
		"  -m, --mode STRING  Selects the operating mode, described\n" +
		"                     at length so that it wraps.\n" +
		"  -h, --help         Show this help message and exit\n"

	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("FormatHelp mismatch (-expected +got):\n%s", diff)
	}
}

func TestFormatHelpCapsTheDescriptionColumn(t *testing.T) {
	t.Parallel()

	parser := &Parser{
		ProgramName: "myapp",
		Width:       80,
		Options: []option.Option{
			option.NewIntOption('n', "n", "Short one", false, nil),
			option.NewStringOption(0, "an-extremely-long-option-name-here", "Long one", false, nil),
		},
	}

	expected := "Usage: myapp [-h] [-n INT] [--an-extremely-long-option-name-here STRING]\n" +
		"\n" +
		"Options:\n" +
		"  -n, --n INT           Short one\n" +
		"      --an-extremely-long-option-name-here STRING\n" +
		"                        Long one\n" +
		"  -h, --help            Show this help message and exit\n"

	if diff := cmp.Diff(expected, parser.FormatHelp()); diff != "" {
		t.Errorf("FormatHelp mismatch (-expected +got):\n%s", diff)
	}
}

func TestParseArgsAttachedShortValue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		args        []string
		wantA       bool
		wantB       bool
		wantInt     int
		wantStrings []string
		wantErr     bool
	}{
		{name: "attached to a lone short option", args: []string{"-n5"}, wantInt: 5},
		{name: "attached after a cluster of flags", args: []string{"-abn5"}, wantA: true, wantB: true, wantInt: 5},
		{name: "attached mid-cluster", args: []string{"-an5"}, wantA: true, wantInt: 5},
		{name: "separate value still works", args: []string{"-abn", "5"}, wantA: true, wantB: true, wantInt: 5},
		{name: "negative attached value", args: []string{"-n-3"}, wantInt: -3},
		{
			// -n takes a value, so the rest of the cluster is that value even though "a" and "b"
			// name real options. Position decides, not whether the letters are known.
			name:    "the remainder is the value, not further names",
			args:    []string{"-na5"},
			wantErr: true,
		},
		{name: "the remainder is the value even when it is all names", args: []string{"-nab"}, wantErr: true},
		{name: "an accumulating option takes an attached value", args: []string{"-sx"}, wantStrings: []string{"x"}},
		{name: "flags still cluster", args: []string{"-ab"}, wantA: true, wantB: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var a, b bool
			var intValue int
			stringsValue := make([]string, 0)

			parser := &Parser{
				ProgramName: "myapp",
				Options: []option.Option{
					option.NewBoolOption('a', "a-flag", "A", false, &a),
					option.NewBoolOption('b', "b-flag", "B", false, &b),
					option.NewIntOption('n', "number", "N", false, &intValue),
					option.NewStringsOption('s', "strings", "S", false, &stringsValue),
				},
			}

			err := parser.ParseArgs(testCase.args)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if a != testCase.wantA {
				t.Errorf("a = %v, want %v", a, testCase.wantA)
			}
			if b != testCase.wantB {
				t.Errorf("b = %v, want %v", b, testCase.wantB)
			}
			if intValue != testCase.wantInt {
				t.Errorf("int = %d, want %d", intValue, testCase.wantInt)
			}
			if diff := cmp.Diff(testCase.wantStrings, stringsValue, diffOpts...); diff != "" {
				t.Errorf("strings mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}

func TestParseArgsAttachedValueDoesNotSetLaterClusterOptions(t *testing.T) {
	t.Parallel()

	// "-nab" makes "ab" the value of -n; -a and -b must be left untouched.
	var a, b bool
	var text string
	parser := &Parser{
		ProgramName: "myapp",
		Options: []option.Option{
			option.NewBoolOption('a', "a-flag", "A", false, &a),
			option.NewBoolOption('b', "b-flag", "B", false, &b),
			option.NewStringOption('n', "name", "N", false, &text),
		},
	}

	if err := parser.ParseArgs([]string{"-nab"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "ab" {
		t.Errorf("name = %q, want %q", text, "ab")
	}
	if a || b {
		t.Errorf("a = %v, b = %v, want both false", a, b)
	}
}

func TestParseArgsAttachedValueIsNotUsedForLongOptions(t *testing.T) {
	t.Parallel()

	// Long options take their value with "=" only; "--number5" is simply an unknown name.
	parser := &Parser{
		ProgramName: "myapp",
		Options:     []option.Option{option.NewIntOption('n', "number", "N", false, nil)},
	}

	if err := parser.ParseArgs([]string{"--number5"}); !errors.Is(err, argumentParserErrors.ErrNameNotFound) {
		t.Errorf("error = %v, want ErrNameNotFound", err)
	}
}

//nolint:paralleltest // Replaces os.Args, so it must not run beside other tests.
func TestParseWithNoArgumentsChecksRequired(t *testing.T) {
	original := os.Args
	t.Cleanup(func() { os.Args = original })
	os.Args = []string{"myapp"}

	var value string
	parser := &Parser{
		ProgramName: "myapp",
		Options:     []option.Option{option.NewStringOption('s', "str", "Required", true, &value)},
	}

	if err := parser.Parse(); !errors.Is(err, argumentParserErrors.ErrMissingRequiredOption) {
		t.Errorf("Parse() error = %v, want ErrMissingRequiredOption", err)
	}
}

func TestParseArgsAbbreviation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		disableAbbrev bool
		args          []string
		wantLong      int
		wantErr       error
	}{
		{name: "an unambiguous prefix is taken by default", args: []string{"--lon", "5"}, wantLong: 5},
		{name: "exact name always works", args: []string{"--long", "5"}, wantLong: 5},
		{name: "prefix with an inline value", args: []string{"--lon=5"}, wantLong: 5},
		{
			name:    "a prefix ambiguous with a sibling",
			args:    []string{"--lo", "5"},
			wantErr: argumentParserErrors.ErrAmbiguousOption,
		},
		{
			name:    "the shortest ambiguous prefix",
			args:    []string{"--l", "5"},
			wantErr: argumentParserErrors.ErrAmbiguousOption,
		},
		{name: "a prefix matching nothing", args: []string{"--zz", "5"}, wantErr: argumentParserErrors.ErrNameNotFound},
		{
			name:    "abbreviation does not apply to short names",
			args:    []string{"-lo", "5"},
			wantErr: argumentParserErrors.ErrNameNotFound,
		},
		{
			name:          "disabled, a prefix is simply unknown",
			disableAbbrev: true,
			args:          []string{"--lon", "5"},
			wantErr:       argumentParserErrors.ErrNameNotFound,
		},
		{
			name:          "disabled, the exact name still works",
			disableAbbrev: true,
			args:          []string{"--long", "5"},
			wantLong:      5,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var longValue, lockValue int
			parser := &Parser{
				ProgramName:   "myapp",
				DisableAbbrev: testCase.disableAbbrev,
				Options: []option.Option{
					option.NewIntOption(0, "long", "Long", false, &longValue),
					option.NewIntOption(0, "lock", "Lock", false, &lockValue),
				},
			}

			err := parser.ParseArgs(testCase.args)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("unexpected error = %v, want %v", err, testCase.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if longValue != testCase.wantLong {
				t.Errorf("long = %d, want %d", longValue, testCase.wantLong)
			}
		})
	}
}

func TestParseArgsAmbiguityNamesTheCandidates(t *testing.T) {
	t.Parallel()

	parser := &Parser{
		ProgramName: "myapp",
		Options: []option.Option{
			option.NewIntOption(0, "long", "Long", false, nil),
			option.NewIntOption(0, "lock", "Lock", false, nil),
		},
	}

	err := parser.ParseArgs([]string{"--l", "5"})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(err, argumentParserErrors.ErrAmbiguousOption) {
		t.Fatalf("error = %v, want ErrAmbiguousOption", err)
	}
	if got := err.Error(); !strings.Contains(got, "lock, long") {
		t.Errorf("error should name the candidates in a stable order, got %q", got)
	}
}

func TestParseArgsChoices(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		args     []string
		wantMode string
		wantErr  error
	}{
		{name: "an offered value", args: []string{"--mode", "fast"}, wantMode: "fast"},
		{name: "an offered value inline", args: []string{"--mode=slow"}, wantMode: "slow"},
		{name: "an offered value attached", args: []string{"-mfast"}, wantMode: "fast"},
		{name: "a value not offered", args: []string{"--mode", "sideways"}, wantErr: argumentParserErrors.ErrInvalidChoice},
		{name: "a value not offered inline", args: []string{"--mode=sideways"}, wantErr: argumentParserErrors.ErrInvalidChoice},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var mode string
			parser := &Parser{
				ProgramName: "myapp",
				Options: []option.Option{
					option.WithChoices(
						option.NewStringOption('m', "mode", "Mode", false, &mode),
						"fast", "slow",
					),
				},
			}

			err := parser.ParseArgs(testCase.args)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("unexpected error = %v, want %v", err, testCase.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mode != testCase.wantMode {
				t.Errorf("mode = %q, want %q", mode, testCase.wantMode)
			}
		})
	}
}

func TestParseArgsOptionalArgument(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		args      []string
		wantColor string
		wantInt   int
		wantErr   error
	}{
		{name: "given with a value", args: []string{"--color", "never"}, wantColor: "never"},
		{name: "given with an inline value", args: []string{"--color=never"}, wantColor: "never"},
		{name: "given alone falls back to the constant", args: []string{"--color"}, wantColor: "always"},
		{
			name:      "given alone before another option",
			args:      []string{"--color", "-i", "1"},
			wantColor: "always",
			wantInt:   1,
		},
		{name: "given alone before the terminator", args: []string{"--color", "--"}, wantColor: "always"},
		{name: "attached to a short name", args: []string{"-cnever"}, wantColor: "never"},
		{name: "short name alone", args: []string{"-c"}, wantColor: "always"},
		{name: "not given at all", args: nil, wantColor: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var color string
			var intValue int
			parser := &Parser{
				ProgramName: "myapp",
				Options: []option.Option{
					option.WithOptionalArgument(
						option.NewStringOption('c', "color", "When to colour", false, &color),
						"always",
					),
					option.NewIntOption('i', "int", "An int option", false, &intValue),
				},
			}

			err := parser.ParseArgs(testCase.args)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("unexpected error = %v, want %v", err, testCase.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if color != testCase.wantColor {
				t.Errorf("color = %q, want %q", color, testCase.wantColor)
			}
			if intValue != testCase.wantInt {
				t.Errorf("int = %d, want %d", intValue, testCase.wantInt)
			}
		})
	}
}

func TestParseArgsDefaults(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		args    []string
		wantInt int
		wantStr string
	}{
		{name: "neither given", args: nil, wantInt: 8080, wantStr: "info"},
		{name: "one given", args: []string{"-p", "9090"}, wantInt: 9090, wantStr: "info"},
		{name: "both given", args: []string{"-p", "9090", "-l", "debug"}, wantInt: 9090, wantStr: "debug"},
		{name: "given the same value as the default", args: []string{"-p", "8080"}, wantInt: 8080, wantStr: "info"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var port int
			var level string
			parser := &Parser{
				ProgramName: "myapp",
				Options: []option.Option{
					option.WithDefault(option.NewIntOption('p', "port", "Port", false, &port), "8080"),
					option.WithDefault(option.NewStringOption('l', "level", "Level", false, &level), "info"),
				},
			}

			if err := parser.ParseArgs(testCase.args); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if port != testCase.wantInt {
				t.Errorf("port = %d, want %d", port, testCase.wantInt)
			}
			if level != testCase.wantStr {
				t.Errorf("level = %q, want %q", level, testCase.wantStr)
			}
		})
	}
}

func TestDefaultDoesNotMaskAMissingRequiredOption(t *testing.T) {
	t.Parallel()

	var value string
	parser := &Parser{
		ProgramName: "myapp",
		Options: []option.Option{
			option.WithDefault(option.NewStringOption('s', "str", "Required", true, &value), "fallback"),
		},
	}

	if err := parser.ParseArgs(nil); !errors.Is(err, argumentParserErrors.ErrMissingRequiredOption) {
		t.Errorf("error = %v, want ErrMissingRequiredOption", err)
	}
}

func TestFormatHelpShowsChoicesAndDefaults(t *testing.T) {
	t.Parallel()

	var mode, color string
	var port int
	parser := &Parser{
		ProgramName: "myapp",
		Width:       80,
		Options: []option.Option{
			option.WithChoices(option.NewStringOption('m', "mode", "Mode", false, &mode), "fast", "slow"),
			option.WithDefault(option.NewIntOption('p', "port", "Port", false, &port), "8080"),
			option.WithOptionalArgument(
				option.NewStringOption('c', "color", "Colour", false, &color),
				"always",
			),
		},
	}

	expected := "Usage: myapp [-h] [-m {fast,slow}] [-p INT] [-c [STRING]]\n" +
		"\n" +
		"Options:\n" +
		"  -m, --mode {fast,slow}\n" +
		"                        Mode\n" +
		"  -p, --port INT        Port (default: 8080)\n" +
		"  -c, --color [STRING]  Colour\n" +
		"  -h, --help            Show this help message and exit\n"

	if diff := cmp.Diff(expected, parser.FormatHelp()); diff != "" {
		t.Errorf("FormatHelp mismatch (-expected +got):\n%s", diff)
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		parser  *Parser
		wantErr error
	}{
		{
			name: "a sound declaration",
			parser: &Parser{Options: []option.Option{
				option.NewIntOption('i', "int", "usage", false, nil),
			}},
		},
		{
			name: "a duplicated name",
			parser: &Parser{Options: []option.Option{
				option.NewIntOption('i', "int", "usage", false, nil),
				option.NewIntOption('i', "other", "usage", false, nil),
			}},
			wantErr: argumentParserErrors.ErrMultipleOptionsWithSameName,
		},
		{
			name: "a duplicated name in a subparser",
			parser: &Parser{
				Options: []option.Option{option.NewIntOption('i', "int", "usage", false, nil)},
				Parsers: []Subparser{
					nil,
					&stubSubparser{command: "opaque"},
					&Parser{Command: "serve", Options: []option.Option{
						option.NewIntOption('p', "port", "usage", false, nil),
						option.NewIntOption('p', "other", "usage", false, nil),
					}},
				},
			},
			wantErr: argumentParserErrors.ErrMultipleOptionsWithSameName,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := testCase.parser.Validate()
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("unexpected error = %v, want %v", err, testCase.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error = %v", err)
			}
		})
	}
}

func TestParseArgsHelpTakesPartInAbbreviation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		competing     bool
		disableAbbrev bool
		disableHelp   bool
		args          []string
		wantHelp      bool
		wantErr       error
	}{
		{name: "exact long name", args: []string{"--help"}, wantHelp: true},
		{name: "exact short name", args: []string{"-h"}, wantHelp: true},
		{name: "an unambiguous prefix reaches help", args: []string{"--hel"}, wantHelp: true},
		{name: "the shortest unambiguous prefix", args: []string{"--h"}, wantHelp: true},
		{
			// An option of the caller's own sharing the prefix makes it ambiguous, exactly as two
			// of the caller's own options would.
			name:      "a competing option makes the prefix ambiguous",
			competing: true,
			args:      []string{"--hel"},
			wantErr:   argumentParserErrors.ErrAmbiguousOption,
		},
		{name: "the exact name still resolves past a competitor", competing: true, args: []string{"--help"}, wantHelp: true},
		{
			name:          "no prefix reaches help once abbreviation is off",
			disableAbbrev: true,
			args:          []string{"--hel"},
			wantErr:       argumentParserErrors.ErrNameNotFound,
		},
		{name: "the exact name works with abbreviation off", disableAbbrev: true, args: []string{"--help"}, wantHelp: true},
		{
			name:        "no prefix reaches a withheld help option",
			disableHelp: true,
			args:        []string{"--hel"},
			wantErr:     argumentParserErrors.ErrNameNotFound,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var output strings.Builder
			var verbose bool
			var helper int

			options := []option.Option{option.NewBoolOption('v', "verbose", "Verbose", false, &verbose)}
			if testCase.competing {
				options = append(options, option.NewIntOption(0, "helper", "Helper", false, &helper))
			}

			parser := &Parser{
				ProgramName:   "myapp",
				Width:         80,
				Output:        &output,
				DisableAbbrev: testCase.disableAbbrev,
				DisableHelp:   testCase.disableHelp,
				Options:       options,
			}

			err := parser.ParseArgs(testCase.args)

			if testCase.wantHelp {
				if !errors.Is(err, argumentParserErrors.ErrHelp) {
					t.Fatalf("error = %v, want ErrHelp", err)
				}
				if output.Len() == 0 {
					t.Error("expected the help message to be written")
				}
				return
			}

			if !errors.Is(err, testCase.wantErr) {
				t.Errorf("error = %v, want %v", err, testCase.wantErr)
			}
			if output.Len() != 0 {
				t.Errorf("expected no help output, got %q", output.String())
			}
		})
	}
}

func TestHelpAmbiguityNamesHelpAmongTheCandidates(t *testing.T) {
	t.Parallel()

	parser := &Parser{
		ProgramName: "myapp",
		Options:     []option.Option{option.NewIntOption(0, "helper", "Helper", false, nil)},
	}

	err := parser.ParseArgs([]string{"--hel"})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(err, argumentParserErrors.ErrAmbiguousOption) {
		t.Fatalf("error = %v, want ErrAmbiguousOption", err)
	}
	if got := err.Error(); !strings.Contains(got, "help, helper") {
		t.Errorf("the automatic help option belongs among the candidates, got %q", got)
	}
}

func TestClaimedHelpNameStaysOutOfTheTables(t *testing.T) {
	t.Parallel()

	// -h belongs to the caller, so only --help answers for help, and a prefix reaches that.
	var host string
	var output strings.Builder
	parser := &Parser{
		ProgramName: "myapp",
		Width:       80,
		Output:      &output,
		Options:     []option.Option{option.NewStringOption('h', "host", "Host", false, &host)},
	}

	if err := parser.ParseArgs([]string{"-h", "example.com"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "example.com" {
		t.Errorf("host = %q, want %q", host, "example.com")
	}

	if err := parser.ParseArgs([]string{"--hel"}); !errors.Is(err, argumentParserErrors.ErrHelp) {
		t.Errorf("error = %v, want ErrHelp", err)
	}
}

func TestFormatError(t *testing.T) {
	t.Parallel()

	parser := &Parser{
		ProgramName: "myapp",
		Width:       80,
		Options: []option.Option{
			option.NewIntOption('i', "int", "An int option", false, nil),
		},
	}

	err := parser.ParseArgs([]string{"leftover"})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	got := parser.FormatError(err)

	if want := "Usage: myapp [-h] [-i INT]\n"; !strings.HasPrefix(got, want) {
		t.Errorf("a report should open with the usage line, got:\n%s", got)
	}
	if !strings.Contains(got, "myapp: error: ") {
		t.Errorf("a report should name the program, got:\n%s", got)
	}
	if !strings.Contains(got, "unexpected argument") {
		t.Errorf("a report should say what went wrong, got:\n%s", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("a report should end in a newline, got:\n%q", got)
	}
}

func TestFormatUsageMatchesTheFirstLineOfHelp(t *testing.T) {
	t.Parallel()

	parser := &Parser{
		ProgramName: "myapp",
		Width:       80,
		Description: "A test application.",
		Options: []option.Option{
			option.NewStringOption('s', "str", "Required", true, nil),
			option.NewIntOption('i', "int", "Optional", false, nil),
		},
	}

	firstLine, _, _ := strings.Cut(parser.FormatHelp(), "\n")
	if got := parser.FormatUsage(); got != firstLine {
		t.Errorf("FormatUsage() = %q, want the first line of help %q", got, firstLine)
	}
}

func TestFormatErrorWithoutAnError(t *testing.T) {
	t.Parallel()

	parser := &Parser{ProgramName: "myapp", Width: 80}

	if got := parser.FormatError(nil); !strings.Contains(got, "myapp: error: \n") {
		t.Errorf("expected an empty reason rather than a panic, got:\n%s", got)
	}
}

func TestSubparserHelpNamesTheCommandPath(t *testing.T) {
	t.Parallel()

	var output strings.Builder

	serve := &Parser{
		Command:     "serve",
		Description: "Serve the thing.",
		Options:     []option.Option{option.NewIntOption('p', "port", "Port", false, nil)},
	}
	parser := &Parser{
		ProgramName: "myapp",
		Width:       80,
		Output:      &output,
		Options:     []option.Option{option.NewBoolOption('v', "verbose", "Verbose", false, nil)},
		Parsers:     []Subparser{serve},
	}

	if err := parser.ParseArgs([]string{"serve", "--help"}); !errors.Is(err, argumentParserErrors.ErrHelp) {
		t.Fatalf("error = %v, want ErrHelp", err)
	}

	// The subparser inherits the parent's Output, so its help lands where the parent's would.
	got := output.String()
	if want := "Usage: myapp serve [-h] [-p INT]\n"; !strings.HasPrefix(got, want) {
		t.Errorf("subcommand help should name the command path:\n%s", got)
	}
	if serve.Width != 80 {
		t.Errorf("subparser Width = %d, want the parent's 80", serve.Width)
	}
}

func TestSubparserKeepsWhatItSetItself(t *testing.T) {
	t.Parallel()

	var parentOutput, childOutput strings.Builder

	serve := &Parser{
		Command:     "serve",
		ProgramName: "chosen-name",
		Width:       40,
		Output:      &childOutput,
		Options:     []option.Option{option.NewIntOption('p', "port", "Port", false, nil)},
	}
	parser := &Parser{
		ProgramName: "myapp",
		Width:       80,
		Output:      &parentOutput,
		Parsers:     []Subparser{serve},
	}

	if err := parser.ParseArgs([]string{"serve", "--help"}); !errors.Is(err, argumentParserErrors.ErrHelp) {
		t.Fatalf("error = %v, want ErrHelp", err)
	}

	if serve.ProgramName != "chosen-name" || serve.Width != 40 {
		t.Errorf("a subparser's own settings must win, got %q and %d", serve.ProgramName, serve.Width)
	}
	if parentOutput.Len() != 0 {
		t.Errorf("expected nothing on the parent's output, got %q", parentOutput.String())
	}
	if !strings.HasPrefix(childOutput.String(), "Usage: chosen-name [-h] [-p INT]\n") {
		t.Errorf("unexpected subcommand help:\n%s", childOutput.String())
	}
}

func TestParseArgsNormalizedChoices(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		args    []string
		want    int
		wantErr error
	}{
		{name: "as written", args: []string{"-n", "7"}, want: 7},
		{name: "leading zeroes", args: []string{"-n", "007"}, want: 7},
		{name: "explicit sign", args: []string{"-n", "+7"}, want: 7},
		{name: "the other choice", args: []string{"-n", "8"}, want: 8},
		{name: "a value not offered", args: []string{"-n", "9"}, wantErr: argumentParserErrors.ErrInvalidChoice},
		{
			// An option that declares choices reports an unreadable value as one that was not
			// offered, which says more than a conversion error would. argparse converts first and
			// reports "invalid int value" instead.
			name:    "not a number at all",
			args:    []string{"-n", "abc"},
			wantErr: argumentParserErrors.ErrInvalidChoice,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var value int
			parser := &Parser{
				ProgramName: "myapp",
				Options: []option.Option{
					option.WithChoices(option.NewIntOption('n', "number", "N", false, &value), "7", "8"),
				},
			}

			err := parser.ParseArgs(testCase.args)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("unexpected error = %v, want %v", err, testCase.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if value != testCase.want {
				t.Errorf("value = %d, want %d", value, testCase.want)
			}
		})
	}
}

func TestParseArgsEmptyStringDefault(t *testing.T) {
	t.Parallel()

	prefix := "unset"
	parser := &Parser{
		ProgramName: "myapp",
		Options: []option.Option{
			option.WithDefault(option.NewStringOption('p', "prefix", "Prefix", false, &prefix), ""),
		},
	}

	if err := parser.ParseArgs(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prefix != "" {
		t.Errorf("prefix = %q, want the declared empty default", prefix)
	}
}

func TestParseArgsExclusiveGroups(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		required bool
		args     []string
		wantErr  error
	}{
		{name: "neither given", args: nil},
		{name: "one given", args: []string{"-q"}},
		{name: "the other given", args: []string{"-v"}},
		{name: "both given", args: []string{"-q", "-v"}, wantErr: argumentParserErrors.ErrMutuallyExclusiveOptions},
		{
			name:    "both given by long name",
			args:    []string{"--quiet", "--verbose"},
			wantErr: argumentParserErrors.ErrMutuallyExclusiveOptions,
		},
		{name: "both given in one cluster", args: []string{"-qv"}, wantErr: argumentParserErrors.ErrMutuallyExclusiveOptions},
		{name: "the same one twice is not a conflict", args: []string{"-v", "-v"}},
		{
			name:     "a required group with neither given",
			required: true,
			args:     nil,
			wantErr:  argumentParserErrors.ErrMissingRequiredOption,
		},
		{name: "a required group satisfied", required: true, args: []string{"-q"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var quiet bool
			var verbose int
			quietOption := option.NewBoolOption('q', "quiet", "Quiet", false, &quiet)
			verboseOption := option.NewCountedOption('v', "verbose", "Verbose", false, &verbose)

			parser := &Parser{
				ProgramName: "myapp",
				Options:     []option.Option{quietOption, verboseOption},
				ExclusiveGroups: []*ExclusiveGroup{
					nil,
					{Options: []option.Option{quietOption, verboseOption}, Required: testCase.required},
				},
			}

			err := parser.ParseArgs(testCase.args)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("unexpected error = %v, want %v", err, testCase.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error = %v", err)
			}
		})
	}
}

func TestFormatHelpExclusiveGroupsAndSections(t *testing.T) {
	t.Parallel()

	var quiet, verbose bool
	var port int
	var host string

	quietOption := option.NewBoolOption('q', "quiet", "Say less", false, &quiet)
	verboseOption := option.NewBoolOption('v', "verbose", "Say more", false, &verbose)
	portOption := option.NewIntOption('p', "port", "Port", false, &port)
	hostOption := option.NewStringOption(0, "host", "Host", false, &host)

	parser := &Parser{
		ProgramName: "myapp",
		Width:       80,
		Options:     []option.Option{quietOption, verboseOption, portOption, hostOption},
		Groups: []*Group{
			{Title: "Network", Options: []option.Option{portOption, hostOption}},
		},
		ExclusiveGroups: []*ExclusiveGroup{
			{Options: []option.Option{quietOption, verboseOption}},
		},
	}

	// Grouping only arranges the help, so the grouped options still appear in the usage line, and
	// every section shares one description column.
	expected := "Usage: myapp [-h] [-q | -v] [-p INT] [--host STRING]\n" +
		"\n" +
		"Options:\n" +
		"  -q, --quiet        Say less\n" +
		"  -v, --verbose      Say more\n" +
		"  -h, --help         Show this help message and exit\n" +
		"\n" +
		"Network:\n" +
		"  -p, --port INT     Port\n" +
		"      --host STRING  Host\n"

	if diff := cmp.Diff(expected, parser.FormatHelp()); diff != "" {
		t.Errorf("FormatHelp mismatch (-expected +got):\n%s", diff)
	}
}

func TestFormatUsageRequiredExclusiveGroup(t *testing.T) {
	t.Parallel()

	fromFile := option.NewStringOption('f', "file", "From a file", false, nil)
	fromStdin := option.NewBoolOption(0, "stdin", "From stdin", false, nil)

	parser := &Parser{
		ProgramName:     "myapp",
		Width:           80,
		Options:         []option.Option{fromFile, fromStdin},
		ExclusiveGroups: []*ExclusiveGroup{{Options: []option.Option{fromFile, fromStdin}, Required: true}},
	}

	if got, want := parser.FormatUsage(), "Usage: myapp [-h] (-f STRING | --stdin)"; got != want {
		t.Errorf("FormatUsage() = %q, want %q", got, want)
	}
}

func TestValidateRejectsAChoiceTheOptionCannotRead(t *testing.T) {
	t.Parallel()

	parser := &Parser{
		Options: []option.Option{
			option.WithChoices(option.NewIntOption('n', "number", "N", false, nil), "7", "eight"),
		},
	}

	if err := parser.Validate(); err == nil {
		t.Error("expected a declaration with an unreadable choice to be rejected")
	}
}

func TestGroupsMustNameDeclaredOptions(t *testing.T) {
	t.Parallel()

	declared := option.NewIntOption('p', "port", "Port", false, nil)
	forgotten := option.NewStringOption(0, "host", "Host", false, nil)

	testCases := []struct {
		name    string
		parser  *Parser
		wantErr bool
	}{
		{
			name: "a group naming a declared option",
			parser: &Parser{
				Options: []option.Option{declared},
				Groups:  []*Group{{Title: "Network", Options: []option.Option{declared}}},
			},
		},
		{
			name: "a group naming an option left out of Options",
			parser: &Parser{
				Options: []option.Option{declared},
				Groups:  []*Group{{Title: "Network", Options: []option.Option{declared, forgotten}}},
			},
			wantErr: true,
		},
		{
			name: "an exclusive group naming an option left out of Options",
			parser: &Parser{
				Options:         []option.Option{declared},
				ExclusiveGroups: []*ExclusiveGroup{{Options: []option.Option{declared, forgotten}}},
			},
			wantErr: true,
		},
		{
			name: "nil members are ignored",
			parser: &Parser{
				Options:         []option.Option{declared},
				Groups:          []*Group{nil, {Title: "Network", Options: []option.Option{nil, declared}}},
				ExclusiveGroups: []*ExclusiveGroup{nil, {Options: []option.Option{nil}}},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			validateErr := testCase.parser.Validate()
			parseErr := testCase.parser.ParseArgs(nil)

			if !testCase.wantErr {
				if validateErr != nil {
					t.Errorf("unexpected Validate error = %v", validateErr)
				}
				if parseErr != nil {
					t.Errorf("unexpected ParseArgs error = %v", parseErr)
				}
				return
			}

			// Both paths must catch it: Validate for a caller that checks at startup, ParseArgs for
			// one that does not.
			if !errors.Is(validateErr, argumentParserErrors.ErrUndeclaredOption) {
				t.Errorf("Validate error = %v, want ErrUndeclaredOption", validateErr)
			}
			if !errors.Is(parseErr, argumentParserErrors.ErrUndeclaredOption) {
				t.Errorf("ParseArgs error = %v, want ErrUndeclaredOption", parseErr)
			}
		})
	}
}

func TestReport(t *testing.T) {
	t.Parallel()

	parser := &Parser{
		ProgramName: "myapp",
		Width:       80,
		Options:     []option.Option{option.NewIntOption('i', "int", "An int option", false, nil)},
	}

	t.Run("a successful parse carries on", func(t *testing.T) {
		t.Parallel()

		message, code, ok := parser.report(nil)
		if !ok || code != 0 || message != "" {
			t.Errorf("report(nil) = (%q, %d, %v), want (\"\", 0, true)", message, code, ok)
		}
	})

	t.Run("help stops without a further word", func(t *testing.T) {
		t.Parallel()

		message, code, ok := parser.report(argumentParserErrors.ErrHelp)
		if ok || code != 0 || message != "" {
			t.Errorf("report(ErrHelp) = (%q, %d, %v), want (\"\", 0, false)", message, code, ok)
		}
	})

	t.Run("a bad invocation is reported and leaves through 2", func(t *testing.T) {
		t.Parallel()

		message, code, ok := parser.report(argumentParserErrors.ErrUnexpectedArgument)
		if ok || code != 2 {
			t.Errorf("report(...) = (_, %d, %v), want (_, 2, false)", code, ok)
		}
		if !strings.HasPrefix(message, "Usage: myapp [-h] [-i INT]\n") {
			t.Errorf("a report should open with the usage line, got:\n%s", message)
		}
		if !strings.Contains(message, "myapp: error: unexpected argument") {
			t.Errorf("a report should say what went wrong, got:\n%s", message)
		}
	})

	t.Run("a wrapped help error still counts as help", func(t *testing.T) {
		t.Parallel()

		// Parse wraps what ParseArgs returns, so report must look through the wrapping.
		wrapped := fmt.Errorf("parse args: %w", argumentParserErrors.ErrHelp)

		message, code, ok := parser.report(wrapped)
		if ok || code != 0 || message != "" {
			t.Errorf("report(wrapped help) = (%q, %d, %v), want (\"\", 0, false)", message, code, ok)
		}
	})
}

// positional builds a positional bound to dst, named and with the given arity.
func positional(dst *[]string, name string, nargs option.Nargs) option.Option {
	return option.WithNargs(
		option.WithMetavar(option.NewStringsOption(0, "", "", false, dst), name),
		nargs,
	)
}

func TestAssignPositionals(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		arities      []option.Nargs
		args         []string
		want         [][]string
		wantLeftover []string
		wantErr      error
	}{
		{
			name:    "one each, in order",
			arities: []option.Nargs{option.NargsOne, option.NargsOne},
			args:    []string{"src", "dst"},
			want:    [][]string{{"src"}, {"dst"}},
		},
		{
			name:    "too few for the required ones",
			arities: []option.Nargs{option.NargsOne, option.NargsOne},
			args:    []string{"src"},
			wantErr: argumentParserErrors.ErrMissingPositional,
		},
		{
			name:         "surplus is left over",
			arities:      []option.Nargs{option.NargsOne},
			args:         []string{"src", "extra"},
			want:         [][]string{{"src"}},
			wantLeftover: []string{"extra"},
		},
		{
			name:    "an optional one yields to a required one",
			arities: []option.Nargs{option.NargsOptional, option.NargsOne},
			args:    []string{"only"},
			want:    [][]string{nil, {"only"}},
		},
		{
			name:    "an optional one takes its share when there is enough",
			arities: []option.Nargs{option.NargsOptional, option.NargsOne},
			args:    []string{"first", "second"},
			want:    [][]string{{"first"}, {"second"}},
		},
		{
			name:    "a variadic one leaves what the later ones need",
			arities: []option.Nargs{option.NargsAtLeastOne, option.NargsOne},
			args:    []string{"a", "b", "c"},
			want:    [][]string{{"a", "b"}, {"c"}},
		},
		{
			name:    "a variadic one must be given something",
			arities: []option.Nargs{option.NargsAtLeastOne, option.NargsOne},
			args:    []string{"only"},
			wantErr: argumentParserErrors.ErrMissingPositional,
		},
		{
			name:    "any number includes none",
			arities: []option.Nargs{option.NargsAny, option.NargsOne},
			args:    []string{"only"},
			want:    [][]string{nil, {"only"}},
		},
		{
			name:    "any number takes the rest",
			arities: []option.Nargs{option.NargsOne, option.NargsAny},
			args:    []string{"a", "b", "c"},
			want:    [][]string{{"a"}, {"b", "c"}},
		},
		{
			name:    "nothing at all",
			arities: []option.Nargs{option.NargsAny},
			args:    nil,
			want:    [][]string{nil},
		},
		{
			name:    "a leading variadic one is fed last",
			arities: []option.Nargs{option.NargsAny, option.NargsOne, option.NargsOne},
			args:    []string{"a", "b"},
			want:    [][]string{nil, {"a"}, {"b"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			targets := make([]*[]string, len(testCase.arities))
			positionals := make([]option.Option, len(testCase.arities))
			for index, nargs := range testCase.arities {
				targets[index] = &[]string{}
				positionals[index] = positional(targets[index], fmt.Sprintf("P%d", index), nargs)
			}

			leftover, err := assignPositionals(positionals, testCase.args)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("unexpected error = %v, want %v", err, testCase.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for index, want := range testCase.want {
				if diff := cmp.Diff(want, *targets[index], diffOpts...); diff != "" {
					t.Errorf("positional %d mismatch (-expected +got):\n%s", index, diff)
				}
			}
			if diff := cmp.Diff(testCase.wantLeftover, leftover, diffOpts...); diff != "" {
				t.Errorf("leftover mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}

func TestParseArgsPositionals(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		args     []string
		wantSrc  string
		wantDst  string
		wantRest []string
		wantFlag bool
		wantErr  error
	}{
		{name: "both given", args: []string{"a", "b"}, wantSrc: "a", wantDst: "b"},
		{name: "interleaved with options", args: []string{"a", "-v", "b"}, wantSrc: "a", wantDst: "b", wantFlag: true},
		{name: "options first", args: []string{"-v", "a", "b"}, wantSrc: "a", wantDst: "b", wantFlag: true},
		{name: "options last", args: []string{"a", "b", "-v"}, wantSrc: "a", wantDst: "b", wantFlag: true},
		{
			name:     "after a terminator they are still positionals",
			args:     []string{"--", "a", "b"},
			wantSrc:  "a",
			wantDst:  "b",
			wantRest: nil,
		},
		{
			name:     "a terminator protects an argument that looks like an option",
			args:     []string{"--", "-v", "b"},
			wantSrc:  "-v",
			wantDst:  "b",
			wantFlag: false,
		},
		{name: "too few", args: []string{"a"}, wantErr: argumentParserErrors.ErrMissingPositional},
		{name: "none at all", args: nil, wantErr: argumentParserErrors.ErrMissingPositional},
		{name: "surplus", args: []string{"a", "b", "c"}, wantSrc: "a", wantDst: "b", wantRest: []string{"c"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var src, dst string
			var flag bool
			var rest []string

			parser := &Parser{
				ProgramName: "myapp",
				Rest:        &rest,
				Options:     []option.Option{option.NewBoolOption('v', "verbose", "Verbose", false, &flag)},
				Positionals: []option.Option{
					option.WithMetavar(option.NewStringOption(0, "", "Source", false, &src), "SRC"),
					option.WithMetavar(option.NewStringOption(0, "", "Target", false, &dst), "DST"),
				},
			}

			err := parser.ParseArgs(testCase.args)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("unexpected error = %v, want %v", err, testCase.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if src != testCase.wantSrc || dst != testCase.wantDst {
				t.Errorf("src = %q, dst = %q, want %q and %q", src, dst, testCase.wantSrc, testCase.wantDst)
			}
			if flag != testCase.wantFlag {
				t.Errorf("verbose = %v, want %v", flag, testCase.wantFlag)
			}
			if diff := cmp.Diff(testCase.wantRest, rest, diffOpts...); diff != "" {
				t.Errorf("rest mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}

func TestParseArgsPositionalsWithoutRestRejectSurplus(t *testing.T) {
	t.Parallel()

	var src string
	parser := &Parser{
		ProgramName: "myapp",
		Positionals: []option.Option{
			option.WithMetavar(option.NewStringOption(0, "", "Source", false, &src), "SRC"),
		},
	}

	if err := parser.ParseArgs([]string{"a", "b"}); !errors.Is(err, argumentParserErrors.ErrUnexpectedArgument) {
		t.Errorf("error = %v, want ErrUnexpectedArgument", err)
	}
}

func TestPositionalChoicesAndDefaults(t *testing.T) {
	t.Parallel()

	t.Run("choices apply", func(t *testing.T) {
		t.Parallel()

		var mode string
		parser := &Parser{
			ProgramName: "myapp",
			Positionals: []option.Option{
				option.WithChoices(
					option.WithMetavar(option.NewStringOption(0, "", "Mode", false, &mode), "MODE"),
					"fast", "slow",
				),
			},
		}

		if err := parser.ParseArgs([]string{"fast"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mode != "fast" {
			t.Errorf("mode = %q, want %q", mode, "fast")
		}

		if err := parser.ParseArgs([]string{"sideways"}); !errors.Is(err, argumentParserErrors.ErrInvalidChoice) {
			t.Errorf("error = %v, want ErrInvalidChoice", err)
		}
	})

	t.Run("an omitted optional one falls back to its default", func(t *testing.T) {
		t.Parallel()

		var target string
		parser := &Parser{
			ProgramName: "myapp",
			Positionals: []option.Option{
				option.WithNargs(
					option.WithDefault(
						option.WithMetavar(option.NewStringOption(0, "", "Target", false, &target), "DST"),
						".",
					),
					option.NargsOptional,
				),
			},
		}

		if err := parser.ParseArgs(nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if target != "." {
			t.Errorf("target = %q, want the declared default", target)
		}
	})
}

func TestCheckPositionalsRejectsTwoVariadics(t *testing.T) {
	t.Parallel()

	var first, second []string
	parser := &Parser{
		ProgramName: "myapp",
		Positionals: []option.Option{
			positional(&first, "FIRST", option.NargsAtLeastOne),
			positional(&second, "SECOND", option.NargsAny),
		},
	}

	if err := parser.Validate(); !errors.Is(err, argumentParserErrors.ErrAmbiguousPositionals) {
		t.Errorf("Validate error = %v, want ErrAmbiguousPositionals", err)
	}
	if err := parser.ParseArgs([]string{"a"}); !errors.Is(err, argumentParserErrors.ErrAmbiguousPositionals) {
		t.Errorf("ParseArgs error = %v, want ErrAmbiguousPositionals", err)
	}
}

func TestFormatHelpPositionals(t *testing.T) {
	t.Parallel()

	var src, dst string
	var files []string
	var verbose bool

	parser := &Parser{
		ProgramName: "myapp",
		Width:       80,
		Description: "Copy things.",
		Options:     []option.Option{option.NewBoolOption('v', "verbose", "Say more", false, &verbose)},
		Positionals: []option.Option{
			option.WithMetavar(option.NewStringOption(0, "", "Where to read from", false, &src), "SRC"),
			option.WithNargs(
				option.WithMetavar(option.NewStringOption(0, "", "Where to write to", false, &dst), "DST"),
				option.NargsOptional,
			),
			option.WithNargs(
				option.WithMetavar(option.NewStringsOption(0, "", "Anything else", false, &files), "EXTRA"),
				option.NargsAny,
			),
		},
	}

	expected := "Usage: myapp [-h] [-v] SRC [DST] [EXTRA...]\n" +
		"\n" +
		"Copy things.\n" +
		"\n" +
		"Arguments:\n" +
		"  SRC            Where to read from\n" +
		"  DST            Where to write to\n" +
		"  EXTRA          Anything else\n" +
		"\n" +
		"Options:\n" +
		"  -v, --verbose  Say more\n" +
		"  -h, --help     Show this help message and exit\n"

	if diff := cmp.Diff(expected, parser.FormatHelp()); diff != "" {
		t.Errorf("FormatHelp mismatch (-expected +got):\n%s", diff)
	}
}

// TestHiddenOptionIsUnlistedButAccepted holds what hidden means, and what it does not. An option
// kept out of the help is still an option: a caller who knows it is there must be able to give it,
// because hiding is about clutter rather than about keeping something out of reach.
func TestHiddenOptionIsUnlistedButAccepted(t *testing.T) {
	t.Parallel()

	var shown string
	var concealed string

	parser := &Parser{
		ProgramName: "thing",
		Description: "Do the thing.",
		Options: []option.Option{
			option.NewStringOption('s', "shown", "An ordinary option.", false, &shown),
			option.WithHidden(
				option.NewStringOption(0, "concealed", "An option kept out of the help.", false, &concealed),
			),
		},
	}

	if err := parser.ParseArgs([]string{"--concealed", "given"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if concealed != "given" {
		t.Errorf("expected the hidden option to be accepted, got %q", concealed)
	}

	help := parser.FormatHelp()

	if strings.Contains(help, "concealed") {
		t.Errorf("expected the hidden option to be unlisted, got:\n%s", help)
	}
	// The one beside it is still listed, so hiding one did not hide the rest.
	if !strings.Contains(help, "--shown") {
		t.Errorf("expected the ordinary option to still be listed, got:\n%s", help)
	}
	// The usage line is where a hidden option is most obviously out of place, because it is one
	// line and every option widens it.
	usageLine := strings.SplitN(help, "\n", 2)[0]
	if strings.Contains(usageLine, "concealed") {
		t.Errorf("expected the usage line to leave the hidden option out, got %q", usageLine)
	}
}

// TestHiddenOptionInAGroupIsUnlisted holds that hiding works wherever an option is declared, not
// only in the ungrouped list.
func TestHiddenOptionInAGroupIsUnlisted(t *testing.T) {
	t.Parallel()

	var shown string
	var concealed string

	shownOption := option.NewStringOption('s', "shown", "An ordinary option.", false, &shown)
	concealedOption := option.WithHidden(
		option.NewStringOption(0, "concealed", "An option kept out of the help.", false, &concealed),
	)

	parser := &Parser{
		ProgramName: "thing",
		Options:     []option.Option{shownOption, concealedOption},
		Groups: []*Group{
			{Title: "Advanced", Options: []option.Option{shownOption, concealedOption}},
		},
	}

	help := parser.FormatHelp()

	if strings.Contains(help, "concealed") {
		t.Errorf("expected the hidden option to be unlisted in its group, got:\n%s", help)
	}
	if !strings.Contains(help, "--shown") {
		t.Errorf("expected the group's other option to still be listed, got:\n%s", help)
	}
}
