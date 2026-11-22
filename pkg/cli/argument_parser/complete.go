// Completions answered by the program itself.
//
// A completion script written by --completion is static: it knows the declaration and nothing a
// program reaches at run time, such as the titles a server holds. For those a shell asks the
// program, by running it with CompleteArgument first and the words of the command line after, and
// the parser answers instead of parsing: it works out where the last word stands, from the same
// declaration and with the same rules as a parse, and writes what that word could become.
//
// The request is the first argument, not an environment variable or a value of --completion,
// because a program that cannot answer must not take it for an ordinary run: a parser from before
// the protocol refuses the unknown option with status 2, while an environment variable is ignored
// by every program that does not look for it, which then runs with half a command line. It is
// recognised only as the very first argument, so a word after "--", or one a Rest takes, is data as
// always.
//
// The answer, on standard output with status 0, is CompleteHeader followed by the directives on the
// first line, then a candidate per line: its value, then a tab and its description where it has
// one. Every candidate is the whole word it completes, "--state=confirmed" for "--state=co", so a
// shell replaces the word with it and needs to know nothing of the syntax. A shell that gets no
// header, another status or nothing in time takes it that the program does not answer.
package argument_parser

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	argumentParserErrors "github.com/altshiftab/utils_go/pkg/cli/argument_parser/errors"
	"github.com/altshiftab/utils_go/pkg/cli/argument_parser/option"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

const (
	// CompleteArgument is the first argument of a request for completions, given as
	// CompleteArgument=VERSION.
	CompleteArgument = "--argument-parser-complete"
	// CompleteVersion is the version of the protocol this parser answers.
	CompleteVersion = "1"
	// CompleteHeader begins every answer. It is referenced only by the code that writes an answer,
	// so it is in a program's binary exactly when the program can answer, which is how a shell
	// tells, without running a program, that it may ask it.
	CompleteHeader = "argument_parser-complete/" + CompleteVersion
	// CompleterTimeout bounds how long a completer may take. A shell waits on the answer with the
	// line half typed.
	CompleterTimeout = 1500 * time.Millisecond
)

// The directives an answer may carry. A shell ignores those it does not know.
const (
	// DirectiveFiles says the declaration knows nothing better than file names for the word.
	DirectiveFiles = "files"
	// DirectiveOrdered says the candidates are ranked, to be shown in the order given.
	DirectiveOrdered = "ordered"
	// DirectiveIds says the values are identifiers, to be shown by their descriptions.
	DirectiveIds = "ids"
)

// Completion is the answer to a request for completions.
type Completion struct {
	Candidates []*option.Candidate
	Directives []string
}

// Write writes the completion as the protocol answers: the header and directives, then a candidate
// per line. A value that a line could not carry, holding a tab or a line break, is left out; a
// description's are made spaces.
func (completion *Completion) Write(writer io.Writer) error {
	if completion == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("completion"))
	}
	if writer == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("writer"))
	}

	var builder strings.Builder

	builder.WriteString(CompleteHeader)
	for _, directive := range completion.Directives {
		builder.WriteString(" ")
		builder.WriteString(directive)
	}
	builder.WriteString("\n")

	for _, candidate := range completion.Candidates {
		if candidate == nil || candidate.Value == "" || strings.ContainsAny(candidate.Value, "\t\r\n") {
			continue
		}

		builder.WriteString(candidate.Value)
		if description := strings.Join(strings.Fields(candidate.Description), " "); description != "" {
			builder.WriteString("\t")
			builder.WriteString(description)
		}
		builder.WriteString("\n")
	}

	if _, err := io.WriteString(writer, builder.String()); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("io write string: %w", err))
	}

	return nil
}

// completeRequest reports whether an argument asks for completions, and the version it asks in.
func completeRequest(argument string) (string, bool) {
	if argument == CompleteArgument {
		return "", true
	}

	version, found := strings.CutPrefix(argument, CompleteArgument+"=")

	return version, found
}

// answerCompletion answers a request for completions of words and returns ErrCompletion, so that
// the program ends as after --help without running. A version this parser does not speak is a
// parse error, reported on stderr with status 2 and nothing on stdout, which is how a later version
// of a shell learns that the program is older.
func (parser *Parser) answerCompletion(version string, words []string) error {
	if version != CompleteVersion {
		return altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: %s=%s (this program answers version %s)",
				argumentParserErrors.ErrUnexpectedOptionValue,
				CompleteArgument,
				version,
				CompleteVersion,
			),
		)
	}

	completion, err := parser.Complete(context.Background(), words)
	if writeErr := completion.Write(parser.getOutput()); writeErr != nil {
		return fmt.Errorf("completion write: %w", writeErr)
	}

	// The candidates a completer could not give are no reason to fail the answer: the shell offers
	// nothing, and the reason goes where a person debugging it will look.
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}

	return argumentParserErrors.ErrCompletion
}

// Complete returns what the last of words could become: the words are a command line after the
// program's name, the last one the word being completed, possibly empty. It never runs the program
// and never fails as a parse would: words it cannot make sense of are passed over. What a completer
// could not give is returned as the error, beside a completion without candidates.
func (parser *Parser) Complete(ctx context.Context, words []string) (*Completion, error) {
	if len(words) == 0 {
		words = []string{""}
	}

	return parser.complete(ctx, words[:len(words)-1], words[len(words)-1])
}

// completionState is what the words before the one being completed established.
type completionState struct {
	tables *nameTables
	// pending is the option still taking values, as a parse would give the next word to.
	pending option.Option
	// pendingCount counts the values pending was given.
	pendingCount int
	seen         *seenNames
	// positionals are the words that named no option, in order.
	positionals []string
	// terminated records a "--", after which every word is a positional.
	terminated bool
}

func (parser *Parser) complete(ctx context.Context, prior []string, current string) (*Completion, error) {
	// A subcommand takes the words after its name, as a parse hands them over. The walk descends
	// into its declaration rather than calling its ParseArgs, which may do anything.
	if len(prior) != 0 {
		for _, found := range subcommands(parser) {
			if found.name != prior[0] {
				continue
			}

			if found.parser == nil {
				return &Completion{Directives: []string{DirectiveFiles}}, nil
			}

			return found.parser.complete(ctx, prior[1:], current)
		}
	}

	var completionShell string
	tableOptions, _, _ := parser.tableOptions(&completionShell)

	tables, err := makeNameTables(tableOptions)
	if err != nil {
		// A declaration a parse would refuse has nothing to offer.
		return &Completion{}, nil
	}

	state := parser.scan(tables, prior)

	if !state.terminated {
		parsed := parseArgument(current)
		if parsed != nil && !parsed.long && !tables.hasNegativeNumberName() && isNegativeNumber(current) {
			parsed = nil
		}

		// A lone dash is an operand to a parse, but typed and completed it asks for the options.
		if current == dash {
			return &Completion{Candidates: parser.optionNames(tableOptions, state, current)}, nil
		}

		if parsed != nil {
			return parser.completeOption(ctx, tableOptions, state, current, parsed)
		}

		if state.pending != nil {
			return parser.completeValues(ctx, state, state.pending, current, "")
		}

		if len(state.positionals) == 0 && len(parser.Parsers) != 0 {
			return &Completion{Candidates: subcommandNames(parser, current)}, nil
		}
	}

	slot := parser.positionalSlot(state)
	if slot == nil {
		if parser.Rest != nil {
			return &Completion{Directives: []string{DirectiveFiles}}, nil
		}

		return &Completion{}, nil
	}

	return parser.completeValues(ctx, state, slot, current, "")
}

// tableOptions returns the options a parse looks names up in: the parser's own, then the automatic
// help and completion options where they are offered.
func (parser *Parser) tableOptions(completionShell *string) ([]option.Option, option.Option, option.Option) {
	options := parser.Options

	helpOption := parser.getHelpOption()
	if helpOption != nil {
		options = append(slices.Clone(options), helpOption)
	}

	completionOption := parser.getCompletionOption(completionShell)
	if completionOption != nil {
		options = append(slices.Clone(options), completionOption)
	}

	return options, helpOption, completionOption
}

// scan reads the words before the one being completed as a parse reads them, setting the values
// that options whose setting has no effect beyond the program are given, so that a completer may
// read them.
func (parser *Parser) scan(tables *nameTables, prior []string) *completionState {
	state := &completionState{tables: tables, seen: makeSeenNames()}
	hasNegativeNumberName := tables.hasNegativeNumberName()

	for index, argument := range prior {
		if argument == "--" {
			state.closePending()
			state.terminated = true
			state.positionals = append(state.positionals, prior[index+1:]...)

			break
		}

		parsed := parseArgument(argument)
		if parsed != nil && !parsed.long && !hasNegativeNumberName && isNegativeNumber(argument) {
			parsed = nil
		}

		if parsed == nil {
			if state.pending == nil {
				state.positionals = append(state.positionals, argument)
				continue
			}

			setPure(state.pending, argument)
			state.pendingCount++
			if !state.pending.GetNargs().IsVariadic() {
				state.pending = nil
			}

			continue
		}

		for nameIndex, name := range parsed.names {
			opt, resolved, ok := parser.resolve(tables, name, parsed.long)
			if !ok {
				// A name a parse would refuse ends what can be read of the argument.
				break
			}

			state.closePending()
			state.seen.record(resolved, parsed.long)

			nargs := opt.GetNargs()
			if !parsed.long && nargs != option.NargsNone && nameIndex+1 < len(parsed.names) {
				setPure(opt, strings.Join(parsed.names[nameIndex+1:], ""))
				break
			}

			switch {
			case parsed.inlineValue != nil:
				if nargs != option.NargsNone {
					setPure(opt, *parsed.inlineValue)
				}
			case nargs == option.NargsNone:
				setPure(opt, "")
			default:
				state.pending = opt
				state.pendingCount = 0
			}
		}
	}

	parser.assignPositionalsPure(state.positionals)

	return state
}

// closePending settles the option still taking values before another is named, as a parse does:
// one whose argument is optional and was given none takes its constant.
func (state *completionState) closePending() {
	if state.pending != nil && state.pendingCount == 0 && state.pending.GetNargs() == option.NargsOptional {
		var constValue string
		if provider, ok := state.pending.(option.ConstProvider); ok {
			constValue = provider.GetConst()
		}

		setPure(state.pending, constValue)
	}

	state.pending = nil
	state.pendingCount = 0
}

// resolve looks an option up by a name as written, a long one by any prefix that names only it,
// and returns the option and its full name. Unlike a parse it reports a name it cannot resolve
// rather than failing.
func (parser *Parser) resolve(tables *nameTables, name string, long bool) (option.Option, string, bool) {
	if opt, ok := tables.lookup(name, long); ok {
		return opt, name, true
	}

	if !long || parser.DisableAbbrev {
		return nil, "", false
	}

	opt, matches := tables.resolveAbbreviation(name)
	if len(matches) != 1 || opt == nil {
		return nil, "", false
	}

	return opt, matches[0], true
}

// setPure gives an option a value if setting it records the value and does nothing else. A value
// the option refuses is passed over: the walk reports nothing.
func setPure(opt option.Option, value string) {
	if _, ok := opt.(option.PureSetter); !ok {
		return
	}

	_ = setOption(opt, value)
}

// slotOf returns the positional the word after count earlier ones goes to: each fixed or optional
// one takes a word, and a variadic one every word from its own on. It is nil past the last.
func (parser *Parser) slotOf(count int) option.Option {
	for _, opt := range parser.Positionals {
		if opt == nil {
			continue
		}

		if opt.GetNargs().IsVariadic() || count == 0 {
			return opt
		}

		count--
	}

	return nil
}

// assignPositionalsPure sets the positionals from the words given them so far, as slotOf assigns
// the words, so that a completer of a later one may read them.
func (parser *Parser) assignPositionalsPure(words []string) {
	for index, word := range words {
		if opt := parser.slotOf(index); opt != nil {
			setPure(opt, word)
		}
	}
}

// positionalSlot returns the positional the word being completed goes to.
func (parser *Parser) positionalSlot(state *completionState) option.Option {
	return parser.slotOf(len(state.positionals))
}

// completeOption completes a word that names options: their names, or the value written into the
// word itself, "--name=value" or a short option's attached "-nvalue".
func (parser *Parser) completeOption(
	ctx context.Context,
	tableOptions []option.Option,
	state *completionState,
	current string,
	parsed *parsedArgument,
) (*Completion, error) {
	if parsed.long {
		if parsed.inlineValue == nil {
			return &Completion{Candidates: parser.optionNames(tableOptions, state, current)}, nil
		}

		opt, _, ok := parser.resolve(state.tables, parsed.names[0], true)
		if !ok || opt.GetNargs() == option.NargsNone {
			return &Completion{}, nil
		}

		written, _, _ := strings.Cut(current, "=")

		return parser.completeValues(ctx, state, opt, *parsed.inlineValue, written+"=")
	}

	for index, name := range parsed.names {
		opt, ok := state.tables.lookup(name, false)
		if !ok {
			return &Completion{}, nil
		}

		if opt.GetNargs() != option.NargsNone && index+1 < len(parsed.names) {
			attached := strings.Join(parsed.names[index+1:], "")
			written := "-" + strings.Join(parsed.names[:index+1], "")

			return parser.completeValues(ctx, state, opt, attached, written)
		}
	}

	if len(parsed.names) == 1 {
		return &Completion{Candidates: parser.optionNames(tableOptions, state, current)}, nil
	}

	return &Completion{}, nil
}

// optionNames offers the options whose names begin with what was typed, each by its long name, or
// its short one where it has no long, or by the short name typed. An option that may not be given
// again, being given already or ruled out by one that was, is not offered, nor is a hidden one.
func (parser *Parser) optionNames(tableOptions []option.Option, state *completionState, typed string) []*option.Candidate {
	excluded := exclusions(parser)

	var candidates []*option.Candidate

	for _, opt := range tableOptions {
		if opt == nil || hidden(opt) {
			continue
		}

		if !repeatable(opt) && state.seen.has(opt.GetShortName(), opt.GetLongName()) {
			continue
		}

		if slices.ContainsFunc(excluded[opt], func(other option.Option) bool {
			return state.seen.has(other.GetShortName(), other.GetLongName())
		}) {
			continue
		}

		var form string
		switch short, long := opt.GetShortName(), opt.GetLongName(); {
		case !strings.HasPrefix(typed, "--") && len(typed) > 1 && short != "":
			form = "-" + short
		case long != "":
			form = "--" + long
		default:
			form = "-" + short
		}

		if !strings.HasPrefix(form, typed) {
			continue
		}

		candidates = append(candidates, &option.Candidate{Value: form, Description: summary(opt.GetUsage())})
	}

	return candidates
}

// subcommandNames offers the subcommands whose names begin with what was typed.
func subcommandNames(parser *Parser, typed string) []*option.Candidate {
	var candidates []*option.Candidate

	for _, found := range subcommands(parser) {
		if strings.HasPrefix(found.name, typed) {
			candidates = append(candidates, &option.Candidate{Value: found.name, Description: summary(found.description)})
		}
	}

	return candidates
}

// completeValues offers the values opt could be given that begin with prefix, each written after
// written, the part of the word before the value. Its choices are offered where it has them, and
// else what its completer gives, once the options not given have their defaults, as a run would
// see them. An option offering neither leaves the shell to offer file names, unless it takes a
// number.
func (parser *Parser) completeValues(
	ctx context.Context,
	state *completionState,
	opt option.Option,
	prefix string,
	written string,
) (*Completion, error) {
	if declared := choices(opt); len(declared) != 0 {
		var candidates []*option.Candidate
		for _, choice := range declared {
			if strings.HasPrefix(choice, prefix) {
				candidates = append(candidates, &option.Candidate{Value: written + choice})
			}
		}

		return &Completion{Candidates: candidates}, nil
	}

	provider, ok := opt.(option.CompleterProvider)
	if !ok || provider.GetCompleter() == nil {
		switch opt.(type) {
		case *option.IntOption, *option.IntsOption, *option.BoolOption, *option.CountedOption:
			return &Completion{}, nil
		default:
			return &Completion{Directives: []string{DirectiveFiles}}, nil
		}
	}

	for _, declared := range parser.Options {
		if declared == nil || state.seen.has(declared.GetShortName(), declared.GetLongName()) {
			continue
		}

		if defaults, ok := declared.(option.DefaultProvider); ok && defaults.GetDefault() != nil {
			setPure(declared, *defaults.GetDefault())
		}
	}

	directives := []string{DirectiveOrdered}
	if provider.GetCompletesIds() {
		directives = append(directives, DirectiveIds)
	}

	completerCtx, cancel := context.WithTimeout(ctx, CompleterTimeout)
	defer cancel()

	found, err := provider.GetCompleter()(completerCtx, prefix)
	if err != nil {
		return &Completion{Directives: directives}, altshiftErrors.New(fmt.Errorf("completer: %w", err), prefix)
	}

	candidates := make([]*option.Candidate, 0, len(found))
	for _, candidate := range found {
		if candidate == nil {
			continue
		}

		candidates = append(candidates, &option.Candidate{Value: written + candidate.Value, Description: candidate.Description})
	}

	return &Completion{Candidates: candidates, Directives: directives}, nil
}
