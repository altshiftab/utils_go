package completion

import (
	"errors"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cli/argument_parser"
	"github.com/altshiftab/utils_go/pkg/cli/argument_parser/option"
)

// wrapped is a subparser that holds a parser rather than being one, as a program that needs to know
// which subcommand ran writes.
type wrapped struct {
	parser *argument_parser.Parser
}

func (w *wrapped) ParseArgs(arguments []string) error { return w.parser.ParseArgs(arguments) }
func (w *wrapped) GetCommand() string                 { return w.parser.Command }
func (w *wrapped) GetDescription() string             { return w.parser.Description }
func (w *wrapped) GetParser() *argument_parser.Parser { return w.parser }

// opaque is a subparser that says nothing about what it holds.
type opaque struct{}

func (*opaque) ParseArgs([]string) error { return nil }
func (*opaque) GetCommand() string       { return "opaque" }

func testParser() *argument_parser.Parser {
	var (
		verbose     bool
		quiet       bool
		format      string
		keyFile     string
		include     []string
		optionalArg string
		domain      string
	)

	verboseOption := option.NewBoolOption('v', "verbose", "Say more about what is happening.", false, &verbose)
	quietOption := option.NewBoolOption('q', "quiet", "Say less.", false, &quiet)

	return &argument_parser.Parser{
		ProgramName: "thing-doer",
		Description: "Do the thing.",
		Options: []option.Option{
			verboseOption,
			quietOption,
			option.WithChoices(
				option.WithMetavar(
					option.NewStringOption('f', "format", "How to write the result. Long explanation here.", false, &format),
					"FORMAT",
				),
				"json", "text",
			),
			option.WithMetavar(
				option.NewStringOption('k', "key-file", "A file holding the key.", false, &keyFile),
				"PATH",
			),
			option.WithNargs(
				option.NewStringsOption('i', "include", "Include a name. May be given more than once.", false, &include),
				option.NargsAny,
			),
			option.WithNargs(
				option.NewStringOption('o', "optional", "Takes a value, or does not.", false, &optionalArg),
				option.NargsOptional,
			),
		},
		ExclusiveGroups: []*argument_parser.ExclusiveGroup{
			{Options: []option.Option{verboseOption, quietOption}},
		},
		Positionals: []option.Option{
			option.WithMetavar(option.NewStringOption(0, "", "The domain.", true, &domain), "DOMAIN"),
		},
	}
}

func write(t *testing.T, parser *argument_parser.Parser, shell string) string {
	t.Helper()

	var builder strings.Builder
	if err := Write(&builder, parser, shell); err != nil {
		t.Fatalf("%s: %v", shell, err)
	}

	return builder.String()
}

// TestZshQuotesBraceExpansionOutsideQuotes holds the bug that a syntax check cannot catch and only
// running the shell revealed: where an option has both a short and a long form, the {-v,--verbose}
// brace expansion must reach the shell unquoted so that it expands into one spec per form. Quoting
// the whole spec as one string leaves the braces literal, and _arguments rejects it with "invalid
// argument" at the moment a caller presses tab.
func TestZshQuotesBraceExpansionOutsideQuotes(t *testing.T) {
	t.Parallel()

	script := write(t, testParser(), Zsh)

	if !strings.Contains(script, `'(-v --verbose -q --quiet)'{-v,--verbose}'[`) {
		t.Errorf("expected the braces outside the quotes, got:\n%s", script)
	}
	// The failing shape, exactly.
	if strings.Contains(script, `'(-v --verbose -q --quiet){-v,--verbose}[`) {
		t.Error("the whole spec was quoted as one string, which _arguments rejects")
	}
}

// TestZshCarriesExclusions holds the part a hand-written completion almost never has, because most
// parsers do not know it: having given --quiet, a caller should not be offered --verbose.
func TestZshCarriesExclusions(t *testing.T) {
	t.Parallel()

	script := write(t, testParser(), Zsh)

	if !strings.Contains(script, "(-v --verbose -q --quiet)") {
		t.Errorf("expected the exclusive group to rule the other option out, got:\n%s", script)
	}
}

func TestZshCarriesChoices(t *testing.T) {
	t.Parallel()

	script := write(t, testParser(), Zsh)

	// The parser validates against the same list, so the completion cannot offer something that
	// would then be rejected.
	if !strings.Contains(script, ":FORMAT:(json text)") {
		t.Errorf("expected the choices to be offered, got:\n%s", script)
	}
}

// TestZshCompletesPathsAsFiles holds the one guess this package makes: a value called PATH is
// completed as a path.
func TestZshCompletesPathsAsFiles(t *testing.T) {
	t.Parallel()

	script := write(t, testParser(), Zsh)

	if !strings.Contains(script, ":PATH:_files") {
		t.Errorf("expected a path to complete as a file, got:\n%s", script)
	}
}

// TestZshRepeatableOptionStaysOffered holds that an option which accumulates is offered again. A
// shell hides an option once it is used, and hiding one that may be given repeatedly would stop a
// caller giving the second of them.
func TestZshRepeatableOptionStaysOffered(t *testing.T) {
	t.Parallel()

	script := write(t, testParser(), Zsh)

	// The star is what says "offer this again"; it takes an argument too, hence the plus.
	if !strings.Contains(script, `'*'{-i,--include}'+[`) {
		t.Errorf("expected the repeatable option to keep being offered, got:\n%s", script)
	}
	// And it must not carry an exclusion list, which would hide it after the first use.
	if strings.Contains(script, `'(-i --include)'{-i,--include}`) {
		t.Error("the repeatable option rules itself out, so it could only be given once")
	}
}

// TestZshOptionalArgumentUsesTheEqualsForm holds the difference between an option whose value may
// be omitted and one whose may not: only the first may be written --optional=value with nothing
// following.
func TestZshOptionalArgumentUsesTheEqualsForm(t *testing.T) {
	t.Parallel()

	script := write(t, testParser(), Zsh)

	if !strings.Contains(script, `{-o,--optional}'=-[`) {
		t.Errorf("expected the optional-argument form, got:\n%s", script)
	}
	// A required value attaches with the plain form instead.
	if !strings.Contains(script, `{-f,--format}'+[`) {
		t.Errorf("expected the required-argument form, got:\n%s", script)
	}
}

// TestDescriptionsAreEscaped holds that a usage string containing a bracket does not end the
// description early and leave the rest read as part of the spec.
func TestDescriptionsAreEscaped(t *testing.T) {
	t.Parallel()

	var value bool
	parser := &argument_parser.Parser{
		ProgramName: "thing",
		Options: []option.Option{
			option.NewBoolOption('x', "awkward", `Takes [brackets], a colon: and an apostrophe's worth.`, false, &value),
		},
	}

	script := write(t, parser, Zsh)

	if strings.Contains(script, "[brackets]") {
		t.Errorf("expected the brackets escaped, got:\n%s", script)
	}
	if !strings.Contains(script, `\[brackets\]`) {
		t.Errorf("expected escaped brackets, got:\n%s", script)
	}
	// A colon separates the parts of a spec, so one in a description has to be escaped too.
	if !strings.Contains(script, `colon\:`) {
		t.Errorf("expected the colon escaped, got:\n%s", script)
	}
	// A single quote cannot be escaped inside single quotes; it closes and reopens them.
	if !strings.Contains(script, `'\''`) {
		t.Errorf("expected the apostrophe quoted, got:\n%s", script)
	}
}

// TestSummaryIsOneLine holds that a shell shows a description in a column beside the option, so
// only the first sentence of a usage written for the help survives.
func TestSummaryIsOneLine(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		usage  string
		expect string
	}{
		{name: "one sentence", usage: "Say more.", expect: "Say more"},
		{name: "only the first sentence", usage: "Say more. Much more. Far too much.", expect: "Say more"},
		{name: "wrapped lines are joined", usage: "Say\n  more\n  about it", expect: "Say more about it"},
		{name: "nothing at all", usage: "", expect: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := summary(testCase.usage); got != testCase.expect {
				t.Errorf("%s: expected %q, got %q", testCase.name, testCase.expect, got)
			}
		})
	}
}

func TestSubcommands(t *testing.T) {
	t.Parallel()

	child := &argument_parser.Parser{
		ProgramName: "thing child",
		Command:     "child",
		Description: "Do a smaller thing.",
	}

	parser := &argument_parser.Parser{
		ProgramName: "thing",
		Parsers: []argument_parser.Subparser{
			child,
			&wrapped{parser: &argument_parser.Parser{
				ProgramName: "thing wrapped",
				Command:     "wrapped",
				Description: "Do a wrapped thing.",
			}},
			// A subparser that says nothing about what it holds is still completed by name.
			&opaque{},
			nil,
		},
	}

	script := write(t, parser, Zsh)

	for _, expected := range []string{"child:Do a smaller thing", "wrapped:Do a wrapped thing", "opaque:"} {
		if !strings.Contains(script, expected) {
			t.Errorf("expected %q in:\n%s", expected, script)
		}
	}

	// A subparser that named its parser gets a function of its own; one that did not cannot.
	if !strings.Contains(script, "_thing_child()") || !strings.Contains(script, "_thing_wrapped()") {
		t.Errorf("expected a function per known subcommand, got:\n%s", script)
	}
	if strings.Contains(script, "_thing_opaque()") {
		t.Errorf("expected no function for a subcommand whose options cannot be seen, got:\n%s", script)
	}
}

func TestBash(t *testing.T) {
	t.Parallel()

	script := write(t, testParser(), Bash)

	for _, expected := range []string{"--verbose", "--format", "--help", "complete -F _thing_doer thing-doer"} {
		if !strings.Contains(script, expected) {
			t.Errorf("expected %q in:\n%s", expected, script)
		}
	}

	// The values of an option that accepts only some are what bash can carry; the descriptions and
	// exclusions are not expressible there.
	if !strings.Contains(script, "'json text'") {
		t.Errorf("expected the choices, got:\n%s", script)
	}
}

// TestBashIdentifierIsUsable holds that a program named with a dash still produces a function name
// a shell will accept.
func TestBashIdentifierIsUsable(t *testing.T) {
	t.Parallel()

	script := write(t, testParser(), Bash)

	if !strings.Contains(script, "_thing_doer()") {
		t.Errorf("expected the dash replaced in the function name, got:\n%s", script)
	}
}

func TestWriteArgumentChecks(t *testing.T) {
	t.Parallel()

	var builder strings.Builder

	if err := Write(nil, testParser(), Zsh); err == nil {
		t.Error("expected a nil writer to be an error")
	}
	if err := Write(&builder, nil, Zsh); err == nil {
		t.Error("expected a nil parser to be an error")
	}
	if err := Write(&builder, testParser(), ""); err == nil {
		t.Error("expected an empty shell to be an error")
	}
	if err := Write(&builder, &argument_parser.Parser{}, Zsh); err == nil {
		t.Error("expected a parser with no program name to be an error")
	}

	err := Write(&builder, testParser(), "fish")
	var unsupported *UnsupportedShellError
	if !errors.As(err, &unsupported) {
		t.Errorf("expected an unsupported shell error, got %v", err)
	}
}

// TestOptionOffersTheShells holds that --completion completes its own values, which is the first
// thing a caller will try.
func TestOptionOffersTheShells(t *testing.T) {
	t.Parallel()

	var shell string
	declared := Option(&shell)

	provider, ok := declared.(option.ChoicesProvider)
	if !ok {
		t.Fatal("expected the option to declare its choices")
	}

	got := provider.GetChoices()
	if len(got) != len(Shells) {
		t.Errorf("expected every shell offered, got %v", got)
	}
}
