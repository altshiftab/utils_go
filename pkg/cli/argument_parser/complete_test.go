package argument_parser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	argumentParserErrors "github.com/altshiftab/utils_go/pkg/cli/argument_parser/errors"
	"github.com/altshiftab/utils_go/pkg/cli/argument_parser/option"
)

// errUnreachable is what a completer that cannot reach its source returns.
var errUnreachable = errors.New("unreachable")

// counted is a wrapping subparser that counts the parses it is asked for.
type counted struct {
	parser *Parser
	calls  *int
}

func (c *counted) ParseArgs(arguments []string) error {
	*c.calls++
	return c.parser.ParseArgs(arguments)
}
func (c *counted) GetCommand() string     { return c.parser.Command }
func (c *counted) GetDescription() string { return c.parser.Description }
func (c *counted) GetParser() *Parser     { return c.parser }

// completeFixture is a program's declaration with the kinds of word a completion meets.
type completeFixture struct {
	parser *Parser
	// keyFile is where the match command's FileOption would create a file, were it set.
	keyFile string
	// wrappedCalls counts the parses the wrapped match command was asked for.
	wrappedCalls int
}

func newCompleteFixture(t *testing.T) *completeFixture {
	t.Helper()

	fixture := &completeFixture{keyFile: filepath.Join(t.TempDir(), "key")}

	var (
		verbose   bool
		questions int
		writeOut  bool
		server    string
		state     string
		titles    []string

		matchVerbose bool
		matchQuiet   bool
		keyFile      os.File
		optional     string
		include      []string
		secret       string
		subject      string
		ref          string
	)

	titleCompleter := func(_ context.Context, prefix string) ([]*option.Candidate, error) {
		var found []*option.Candidate
		for _, title := range []string{"Secrets of the Neanderthals", "Black Bag", "Secret Agent"} {
			if strings.HasPrefix(title, prefix) {
				found = append(found, &option.Candidate{Value: title, Description: server})
			}
		}
		return found, nil
	}

	subjectCompleter := func(_ context.Context, prefix string) ([]*option.Candidate, error) {
		var found []*option.Candidate
		for id, title := range map[string]string{"s1": "Hamnet (2025)"} {
			if strings.HasPrefix(id, prefix) {
				found = append(found, &option.Candidate{Value: id, Description: title})
			}
		}
		return found, nil
	}

	refCompleter := func(_ context.Context, _ string) ([]*option.Candidate, error) {
		return []*option.Candidate{{Value: "movie:1-" + subject}, {Value: "none"}}, nil
	}

	quiz := &Parser{
		Command:     "quiz",
		Description: "Make a quiz. On a title.",
		Options: []option.Option{
			option.NewIntOption('n', "questions", "How many questions.", false, &questions),
			option.NewBoolOption(0, "print", "Write it out.", false, &writeOut),
			option.WithDefault(option.NewStringOption(0, "server", "The server.", false, &server), "https://default"),
			option.WithChoices(option.NewStringOption('s', "state", "Only in this state.", false, &state), "auto", "confirmed", "none"),
		},
		Positionals: []option.Option{
			option.WithCompleter(
				option.WithNargs(option.WithMetavar(option.NewStringsOption(0, "", "The title.", false, &titles), "TITLE"), option.NargsAny),
				titleCompleter,
			),
		},
	}

	verboseOption := option.NewBoolOption('v', "verbose", "Say more.", false, &matchVerbose)
	quietOption := option.NewBoolOption('q', "quiet", "Say less.", false, &matchQuiet)

	match := &Parser{
		Command:     "match",
		Description: "Confirm a match.",
		Options: []option.Option{
			verboseOption,
			quietOption,
			option.NewFileOptionExtra('k', "key-file", "A key.", false, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600, &keyFile),
			option.WithOptionalArgument(option.NewStringOption('o', "optional", "Maybe a value.", false, &optional), "c"),
			option.WithNargs(option.NewStringsOption('i', "include", "Include one.", false, &include), option.NargsAny),
			option.WithHidden(option.NewStringOption(0, "secret", "Hidden.", false, &secret)),
		},
		ExclusiveGroups: []*ExclusiveGroup{{Options: []option.Option{verboseOption, quietOption}}},
		Positionals: []option.Option{
			option.WithIdCompleter(option.WithMetavar(option.NewStringOption(0, "", "The subject.", true, &subject), "SUBJECT"), subjectCompleter),
			option.WithCompleter(option.WithMetavar(option.NewStringOption(0, "", "The ref.", true, &ref), "REF"), refCompleter),
		},
	}

	fixture.parser = &Parser{
		ProgramName: "prog",
		Options:     []option.Option{option.NewBoolOption('v', "verbose", "Say more.", false, &verbose)},
		Parsers: []Subparser{
			quiz,
			&counted{parser: match, calls: &fixture.wrappedCalls},
			&opaque{},
		},
	}

	return fixture
}

func values(completion *Completion) []string {
	var found []string
	for _, candidate := range completion.Candidates {
		found = append(found, candidate.Value)
	}
	return found
}

func TestComplete(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		words        []string
		want         []string
		directives   []string
		descriptions []string
	}{
		{name: "subcommands", words: []string{""}, want: []string{"quiz", "match", "opaque"}, descriptions: []string{"Make a quiz", "Confirm a match", ""}},
		{name: "subcommand prefix", words: []string{"q"}, want: []string{"quiz"}},
		{name: "top options", words: []string{"--"}, want: []string{"--verbose", "--help"}},
		{name: "no subcommand after an option", words: []string{"-v", ""}},
		{name: "opaque subcommand", words: []string{"opaque", ""}, directives: []string{DirectiveFiles}},
		{name: "unknown word", words: []string{"nosuch", ""}},
		{name: "completer", words: []string{"quiz", ""}, want: []string{"Secrets of the Neanderthals", "Black Bag", "Secret Agent"}, directives: []string{DirectiveOrdered}},
		{name: "completer prefix", words: []string{"quiz", "Secrets"}, want: []string{"Secrets of the Neanderthals"}, directives: []string{DirectiveOrdered}},
		{name: "completer sees default", words: []string{"quiz", "Black"}, want: []string{"Black Bag"}, directives: []string{DirectiveOrdered}, descriptions: []string{"https://default"}},
		{name: "completer sees earlier value", words: []string{"quiz", "--server", "https://other", "Black"}, want: []string{"Black Bag"}, directives: []string{DirectiveOrdered}, descriptions: []string{"https://other"}},
		{name: "variadic positional again", words: []string{"quiz", "Black Bag", "Sec"}, want: []string{"Secrets of the Neanderthals", "Secret Agent"}, directives: []string{DirectiveOrdered}},
		{name: "long name prefix", words: []string{"quiz", "--q"}, want: []string{"--questions"}},
		{name: "lone dash", words: []string{"quiz", "-"}, want: []string{"--questions", "--print", "--server", "--state", "--help"}},
		{name: "short name", words: []string{"quiz", "-n"}, want: []string{"-n"}},
		{name: "unknown short name", words: []string{"quiz", "-z"}},
		{name: "inline value", words: []string{"quiz", "--state=co"}, want: []string{"--state=confirmed"}},
		{name: "inline value of abbreviation", words: []string{"quiz", "--st=co"}, want: []string{"--st=confirmed"}},
		{name: "attached short value", words: []string{"quiz", "-sco"}, want: []string{"-sconfirmed"}},
		{name: "pending choices", words: []string{"quiz", "--state", ""}, want: []string{"auto", "confirmed", "none"}},
		{name: "pending number", words: []string{"quiz", "-n", ""}},
		{name: "attached number taken", words: []string{"quiz", "-n5", "Black"}, want: []string{"Black Bag"}, directives: []string{DirectiveOrdered}},
		{name: "given option not offered again", words: []string{"quiz", "--print", "--p"}},
		{name: "after terminator", words: []string{"quiz", "--", "--state=co"}, directives: []string{DirectiveOrdered}},
		{name: "id completer", words: []string{"match", ""}, want: []string{"s1"}, directives: []string{DirectiveOrdered, DirectiveIds}, descriptions: []string{"Hamnet (2025)"}},
		{name: "second positional sees first", words: []string{"match", "s1", ""}, want: []string{"movie:1-s1", "none"}, directives: []string{DirectiveOrdered}},
		{name: "past the positionals", words: []string{"match", "s1", "none", ""}},
		{name: "exclusive and given options left out", words: []string{"match", "-v", "-"}, want: []string{"--key-file", "--optional", "--include", "--help"}},
		{name: "repeatable option offered again", words: []string{"match", "-i", "a", "--inc"}, want: []string{"--include"}},
		{name: "file option", words: []string{"match", "-k", ""}, directives: []string{DirectiveFiles}},
		{name: "optional argument closed by an option", words: []string{"match", "-o", "-v", "s"}, want: []string{"s1"}, directives: []string{DirectiveOrdered, DirectiveIds}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fixture := newCompleteFixture(t)

			completion, err := fixture.parser.Complete(t.Context(), testCase.words)
			if err != nil {
				t.Fatalf("complete: %v", err)
			}

			if got := values(completion); !slices.Equal(got, testCase.want) {
				t.Errorf("candidates: got %q, want %q", got, testCase.want)
			}

			if !slices.Equal(completion.Directives, testCase.directives) {
				t.Errorf("directives: got %q, want %q", completion.Directives, testCase.directives)
			}

			for index, description := range testCase.descriptions {
				if index >= len(completion.Candidates) || completion.Candidates[index].Description != description {
					t.Errorf("description %d: got %+v, want %q", index, completion.Candidates, description)
				}
			}
		})
	}
}

func TestCompleteCompleterError(t *testing.T) {
	t.Parallel()

	var title string
	parser := &Parser{
		ProgramName: "prog",
		Positionals: []option.Option{
			option.WithCompleter(option.NewStringOption(0, "", "A title.", false, &title), func(context.Context, string) ([]*option.Candidate, error) {
				return nil, errUnreachable
			}),
		},
	}

	var output bytes.Buffer
	parser.Output = &output

	err := parser.ParseArgs([]string{CompleteArgument + "=1", "Ham"})
	if !errors.Is(err, argumentParserErrors.ErrCompletion) {
		t.Fatalf("parse args: got %v, want the completion answered", err)
	}

	if got, want := output.String(), CompleteHeader+" "+DirectiveOrdered+"\n"; got != want {
		t.Errorf("output: got %q, want %q", got, want)
	}
}

func TestCompleteRequest(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args []string
		// want is the output, empty where nothing may be written.
		want    string
		status  int
		answers bool
	}{
		{
			name:    "answered",
			args:    []string{CompleteArgument + "=1", "quiz", "Secrets"},
			want:    CompleteHeader + " ordered\nSecrets of the Neanderthals\thttps://default\n",
			answers: true,
		},
		{
			name:    "missing current word",
			args:    []string{CompleteArgument + "=1"},
			want:    CompleteHeader + "\nquiz\tMake a quiz\nmatch\tConfirm a match\nopaque\n",
			answers: true,
		},
		{name: "unknown version", args: []string{CompleteArgument + "=2", "quiz", ""}, status: 2},
		{name: "no version", args: []string{CompleteArgument, "quiz", ""}, status: 2},
		{
			name:    "answered by a subcommand reached first",
			args:    []string{"quiz", CompleteArgument + "=1", "Black"},
			want:    CompleteHeader + " ordered\nBlack Bag\thttps://default\n",
			answers: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fixture := newCompleteFixture(t)

			var output bytes.Buffer
			fixture.parser.Output = &output

			err := fixture.parser.ParseArgs(testCase.args)
			if answered := errors.Is(err, argumentParserErrors.ErrCompletion); answered != testCase.answers {
				t.Fatalf("parse args: got %v, answered %v, want %v", err, answered, testCase.answers)
			}

			if got := output.String(); got != testCase.want {
				t.Errorf("output: got %q, want %q", got, testCase.want)
			}

			if _, status, _ := fixture.parser.report(err); status != testCase.status {
				t.Errorf("status: got %d, want %d", status, testCase.status)
			}
		})
	}
}

// TestCompleteRequestRunsNothing pins that answering parses nothing a run would: a file option is
// not opened, a wrapping subparser is not asked to parse, and a missing required argument is no
// error.
func TestCompleteRequestRunsNothing(t *testing.T) {
	t.Parallel()

	fixture := newCompleteFixture(t)
	fixture.parser.Output = &bytes.Buffer{}

	err := fixture.parser.ParseArgs([]string{CompleteArgument + "=1", "match", "-k", fixture.keyFile, ""})
	if !errors.Is(err, argumentParserErrors.ErrCompletion) {
		t.Fatalf("parse args: got %v, want the completion answered", err)
	}

	if _, statErr := os.Stat(fixture.keyFile); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("key file: got %v, want it never created", statErr)
	}

	if fixture.wrappedCalls != 0 {
		t.Errorf("wrapped parses: got %d, want 0", fixture.wrappedCalls)
	}

	// The same file option, given in a real parse, does create the file: the check above is not
	// vacuous.
	if err := fixture.parser.ParseArgs([]string{"match", "-k", fixture.keyFile, "s1", "none"}); err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if _, statErr := os.Stat(fixture.keyFile); statErr != nil {
		t.Errorf("key file after a real parse: %v", statErr)
	}
}

func TestCompleteRequestAfterTerminatorIsData(t *testing.T) {
	t.Parallel()

	var rest []string
	parser := &Parser{ProgramName: "prog", Rest: &rest, Output: &bytes.Buffer{}}

	if err := parser.ParseArgs([]string{"--", CompleteArgument + "=1", "x"}); err != nil {
		t.Fatalf("parse args: %v", err)
	}

	if want := []string{CompleteArgument + "=1", "x"}; !slices.Equal(rest, want) {
		t.Errorf("rest: got %q, want %q", rest, want)
	}
}

func TestValidateReservesCompleteArgument(t *testing.T) {
	t.Parallel()

	var value string
	parser := &Parser{Options: []option.Option{option.NewStringOption(0, "argument-parser-complete", "", false, &value)}}

	if err := parser.Validate(); !errors.Is(err, argumentParserErrors.ErrReservedName) {
		t.Errorf("validate: got %v, want %v", err, argumentParserErrors.ErrReservedName)
	}
}

func TestCompletionWrite(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		completion *Completion
		want       string
	}{
		{name: "empty", completion: &Completion{}, want: CompleteHeader + "\n"},
		{
			name:       "directives",
			completion: &Completion{Directives: []string{DirectiveOrdered, DirectiveIds}, Candidates: []*option.Candidate{{Value: "a"}}},
			want:       CompleteHeader + " ordered ids\na\n",
		},
		{
			name: "values a line cannot carry are left out",
			completion: &Completion{Candidates: []*option.Candidate{
				{Value: "tab\there"}, {Value: "line\nbreak"}, {Value: ""}, nil, {Value: "kept", Description: "two\tparts\nand  more"},
			}},
			want: CompleteHeader + "\nkept\ttwo parts and more\n",
		},
		{
			name: "control characters reach no terminal",
			completion: &Completion{Candidates: []*option.Candidate{
				{Value: "esc\x1b[31mred"}, {Value: "csi\u009b31m"}, {Value: "bell\a"},
				{Value: "title", Description: "set\x1b]0;title\a and \x1b[2Jclear"},
			}},
			want: CompleteHeader + "\ntitle\tset ]0;title and [2Jclear\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			if err := testCase.completion.Write(&output); err != nil {
				t.Fatalf("write: %v", err)
			}

			if got := output.String(); got != testCase.want {
				t.Errorf("output: got %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestCompleteAgreesWithParse pins that the word being completed is offered the values of the
// option or positional a parse gives that word to.
func TestCompleteAgreesWithParse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		words []string
		want  string
	}{
		{name: "short option", words: []string{"-a", "x"}, want: "a"},
		{name: "long option", words: []string{"--bb", "x"}, want: "bb"},
		{name: "abbreviated long option", words: []string{"--b", "x"}, want: "bb"},
		{name: "variadic option", words: []string{"-c", "y", "x"}, want: "c"},
		{name: "terminator ends the variadic option", words: []string{"-c", "y", "--", "x"}, want: "first"},
		{name: "first positional", words: []string{"x"}, want: "first"},
		{name: "after an option's value", words: []string{"-a", "v", "x"}, want: "first"},
		{name: "second positional", words: []string{"p", "x"}, want: "second"},
		{name: "second positional again", words: []string{"p", "q", "x"}, want: "second"},
		{name: "attached value then positional", words: []string{"-av", "x"}, want: "first"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				a, bb, first string
				c, second    []string
				completed    string
			)

			recording := func(name string) option.Completer {
				return func(context.Context, string) ([]*option.Candidate, error) {
					completed = name
					return nil, nil
				}
			}

			parser := &Parser{
				ProgramName: "prog",
				Options: []option.Option{
					option.WithCompleter(option.NewStringOption('a', "", "", false, &a), recording("a")),
					option.WithCompleter(option.NewStringOption(0, "bb", "", false, &bb), recording("bb")),
					option.WithCompleter(option.WithNargs(option.NewStringsOption('c', "", "", false, &c), option.NargsAny), recording("c")),
				},
				Positionals: []option.Option{
					option.WithCompleter(option.WithNargs(option.NewStringOption(0, "", "", false, &first), option.NargsOptional), recording("first")),
					option.WithCompleter(option.WithNargs(option.NewStringsOption(0, "", "", false, &second), option.NargsAny), recording("second")),
				},
			}

			if _, err := parser.Complete(t.Context(), testCase.words); err != nil {
				t.Fatalf("complete: %v", err)
			}
			if completed != testCase.want {
				t.Errorf("completed: got %q, want %q", completed, testCase.want)
			}

			a, bb, first, c, second = "", "", "", nil, nil
			if err := parser.ParseArgs(testCase.words); err != nil {
				t.Fatalf("parse args: %v", err)
			}

			parsed := map[string]bool{
				"a":      a == "x",
				"bb":     bb == "x",
				"first":  first == "x",
				"c":      slices.Contains(c, "x"),
				"second": slices.Contains(second, "x"),
			}
			if !parsed[testCase.want] {
				t.Errorf("parse gave the word elsewhere: %v", parsed)
			}
		})
	}
}

// TestCompleteInBinary builds a program on the parser and asks it, as a shell does: the binary
// holds the header a shell looks for, the answer comes back, and the program's body never runs.
func TestCompleteInBinary(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("builds a program")
	}

	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go tool")
	}

	directory := t.TempDir()
	binary := filepath.Join(directory, "fixture")

	build := exec.CommandContext(t.Context(), goTool, "build", "-o", binary, "./testdata/complete_fixture")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, output)
	}

	contents, err := os.ReadFile(binary)
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}
	if !bytes.Contains(contents, []byte(CompleteHeader)) {
		t.Errorf("binary lacks %q", CompleteHeader)
	}

	sentinel := filepath.Join(directory, "ran")

	request := exec.CommandContext(t.Context(), binary, CompleteArgument+"=1", "Se")
	request.Env = append(os.Environ(), "FIXTURE_SENTINEL="+sentinel)
	output, err := request.Output()
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if got, want := string(output), CompleteHeader+" ordered\nSecrets\n"; got != want {
		t.Errorf("answer: got %q, want %q", got, want)
	}
	if _, statErr := os.Stat(sentinel); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("sentinel: got %v, want the body never run", statErr)
	}

	// An ordinary run does reach the body, so the check above is not vacuous.
	run := exec.CommandContext(t.Context(), binary, "Secrets")
	run.Env = append(os.Environ(), "FIXTURE_SENTINEL="+sentinel)
	if output, err := run.CombinedOutput(); err != nil {
		t.Fatalf("run: %v\n%s", err, output)
	}
	if _, statErr := os.Stat(sentinel); statErr != nil {
		t.Errorf("sentinel after a run: %v", statErr)
	}
}

// sideEffecting is an option of a program's own that embeds a plain one and sets differently, as a
// program writes to act on its value.
type sideEffecting struct {
	*option.StringOption
	calls *int
}

func (opt *sideEffecting) Set(value string) error {
	*opt.calls++
	return opt.StringOption.Set(value)
}

// TestCompleteSetsOnlyPlainOptions pins that the walk sets only the option kinds known to record a
// value and nothing more: not one of a program's own, even embedding a plain kind.
func TestCompleteSetsOnlyPlainOptions(t *testing.T) {
	t.Parallel()

	var value, title string
	calls := 0

	parser := &Parser{
		ProgramName: "prog",
		Options: []option.Option{
			&sideEffecting{StringOption: option.NewStringOption(0, "wrapped", "", false, &value), calls: &calls},
		},
		Positionals: []option.Option{
			option.WithCompleter(option.NewStringOption(0, "", "", false, &title), func(context.Context, string) ([]*option.Candidate, error) {
				return nil, nil
			}),
		},
	}

	if _, err := parser.Complete(t.Context(), []string{"--wrapped", "x", ""}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if calls != 0 || value != "" {
		t.Errorf("own option set %d times to %q during a completion, want never", calls, value)
	}

	// A parse does set it, so the check above is not vacuous.
	if err := parser.ParseArgs([]string{"--wrapped", "x", "t"}); err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if calls != 1 {
		t.Errorf("own option set %d times by a parse, want 1", calls)
	}
}

// TestCompleteReservesForLaterPositionals pins that a word goes to the positional a parse gives it
// to, an optional one going without while a later required one needs the word.
func TestCompleteReservesForLaterPositionals(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		words []string
		// want is the positional whose completer is asked, and seen what the earlier ones held then.
		want string
		seen string
	}{
		{name: "the only word goes to the required one", words: []string{""}, want: "b"},
		{name: "with two words the optional one takes the first", words: []string{"x", ""}, want: "b", seen: "a=x b="},
		{name: "two required after an optional", words: []string{"first", ""}, want: "c", seen: "a= b=first"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var a, b, c, completed, seen string
			recording := func(name string) option.Completer {
				return func(context.Context, string) ([]*option.Candidate, error) {
					completed = name
					seen = "a=" + a + " b=" + b
					return nil, nil
				}
			}

			positionals := []option.Option{
				option.WithCompleter(option.WithNargs(option.NewStringOption(0, "", "", false, &a), option.NargsOptional), recording("a")),
				option.WithCompleter(option.NewStringOption(0, "", "", true, &b), recording("b")),
			}
			if testCase.want == "c" {
				positionals = append(positionals, option.WithCompleter(option.NewStringOption(0, "", "", true, &c), recording("c")))
			}

			parser := &Parser{ProgramName: "prog", Positionals: positionals}

			if _, err := parser.Complete(t.Context(), testCase.words); err != nil {
				t.Fatalf("complete: %v", err)
			}

			if completed != testCase.want {
				t.Errorf("completed: got %q, want %q", completed, testCase.want)
			}
			if testCase.seen != "" && seen != testCase.seen {
				t.Errorf("seen: got %q, want %q", seen, testCase.seen)
			}
		})
	}
}

// TestCompleteValueInWordSeesWhatParseSet pins that a value written into the word being completed,
// "--name=value" or an attached "-nvalue", is completed once what a parse does with the word up to
// the value is done, as for a value that is a word of its own: an optional argument left without
// one takes its constant, the flags before an attached value are set, and the option named takes
// no default.
func TestCompleteValueInWordSeesWhatParseSet(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		words []string
		// want is what the options held when the completer was asked.
		want string
	}{
		{name: "inline value after an optional argument", words: []string{"-o", "--into="}, want: `o="c" v=false tags=["dflt"]`},
		{name: "separate value after an optional argument", words: []string{"-o", "--into", ""}, want: `o="c" v=false tags=["dflt"]`},
		{name: "attached value after an optional argument", words: []string{"-o", "-tpre"}, want: `o="c" v=false tags=["dflt"]`},
		{name: "attached value after a flag in its cluster", words: []string{"-vtpre"}, want: `o="" v=true tags=["dflt"]`},
		{name: "separate value after a cluster", words: []string{"-vt", "pre"}, want: `o="" v=true tags=["dflt"]`},
		{name: "inline value of an option with a default", words: []string{"--tag=pre"}, want: `o="" v=false tags=[]`},
		{name: "inline value of an abbreviation with a default", words: []string{"--ta=pre"}, want: `o="" v=false tags=[]`},
		{name: "separate value of an option with a default", words: []string{"--tag", "pre"}, want: `o="" v=false tags=[]`},
		{name: "attached value of an option with a default", words: []string{"-vgpre"}, want: `o="" v=true tags=[]`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				optional, into, seen string
				verbose              bool
				tags                 []string
			)

			recording := func(context.Context, string) ([]*option.Candidate, error) {
				seen = fmt.Sprintf("o=%q v=%t tags=%q", optional, verbose, tags)
				return nil, nil
			}

			parser := &Parser{
				ProgramName: "prog",
				Options: []option.Option{
					option.WithOptionalArgument(option.NewStringOption('o', "", "", false, &optional), "c"),
					option.NewBoolOption('v', "", "", false, &verbose),
					option.WithCompleter(option.NewStringOption('t', "into", "", false, &into), recording),
					option.WithCompleter(option.WithDefault(option.NewStringsOption('g', "tag", "", false, &tags), "dflt"), recording),
				},
			}

			if _, err := parser.Complete(t.Context(), testCase.words); err != nil {
				t.Fatalf("complete: %v", err)
			}
			if seen != testCase.want {
				t.Errorf("completer saw %s, want %s", seen, testCase.want)
			}
		})
	}
}

// TestCompleteOffersAccumulatingOptionsAgain pins that an option given already is offered again
// where a repeat adds to what it holds, whatever its arity: a counter, and a list taking one value
// per occurrence, as well as one taking any number. A single value is not, even with an arity that
// takes several, since a repeat only replaces it.
func TestCompleteOffersAccumulatingOptionsAgain(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		words []string
		want  []string
	}{
		{name: "counter", words: []string{"-v", "--v"}, want: []string{"--verbose"}},
		{name: "counter in a cluster", words: []string{"-vv", "--v"}, want: []string{"--verbose"}},
		{name: "counter by its short name", words: []string{"-v", "-v"}, want: []string{"-v"}},
		{name: "strings one per occurrence", words: []string{"-r", "a", "--r"}, want: []string{"--root"}},
		{name: "ints one per occurrence", words: []string{"-n", "1", "--n"}, want: []string{"--number"}},
		{name: "strings any number", words: []string{"-l", "a", "b", "--l"}, want: []string{"--label"}},
		{name: "own option with a variadic arity", words: []string{"--own", "a", "--o"}, want: []string{"--own"}},
		{name: "single value with a variadic arity", words: []string{"-w", "a", "--w"}},
		{name: "flag", words: []string{"-b", "--b"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				count          int
				numbers        []int
				roots, labels  []string
				word, ownValue string
				flag           bool
				ownCalls       int
			)

			own := &sideEffecting{StringOption: option.NewStringOption(0, "own", "", false, &ownValue), calls: &ownCalls}
			own.SetNargs(option.NargsAny)

			parser := &Parser{
				ProgramName: "prog",
				Options: []option.Option{
					option.NewCountedOption('v', "verbose", "", false, &count),
					option.WithNargs(option.NewStringsOption('r', "root", "", false, &roots), option.NargsOne),
					option.WithNargs(option.NewIntsOption('n', "number", "", false, &numbers), option.NargsOne),
					option.NewStringsOption('l', "label", "", false, &labels),
					option.WithNargs(option.NewStringOption('w', "word", "", false, &word), option.NargsAny),
					option.NewBoolOption('b', "bool", "", false, &flag),
					own,
				},
			}

			completion, err := parser.Complete(t.Context(), testCase.words)
			if err != nil {
				t.Fatalf("complete: %v", err)
			}
			if got := values(completion); !slices.Equal(got, testCase.want) {
				t.Errorf("candidates: got %q, want %q", got, testCase.want)
			}
		})
	}
}
