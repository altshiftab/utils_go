// Package completion writes shell completions from a parser's own declaration.
//
// A parser already knows everything a completion needs: the options and what each one is for,
// whether it takes an argument, the values it accepts, which options rule out which others, and
// what the subcommands are. A hand-written completion has to be told all of that again, and drifts
// the first time an option is added. Generating it means the completion cannot disagree with what
// the program accepts, because both are read from the same declaration.
//
// The generated script is static: it encodes the declaration and calls nothing back. That keeps
// completion working when the binary is slow to start, or absent, and makes the output something a
// package can ship. Values that are only knowable at run time are the exception, and belong to the
// program rather than here.
package completion

import (
	"fmt"
	"io"
	"strings"

	"github.com/altshiftab/utils_go/pkg/cli/argument_parser"
	"github.com/altshiftab/utils_go/pkg/cli/argument_parser/option"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

// The shells that can be written.
const (
	Zsh  = "zsh"
	Bash = "bash"
)

// Shells are the shells this package writes, in the order the help lists them.
//
// zsh comes first because it is the one that can express the whole declaration: descriptions
// against each option, the values an option accepts, and options that rule one another out. bash's
// completion is a coarser thing and says less, which is a limit of the shell rather than of what is
// known here.
var Shells = []string{Zsh, Bash}

// UnsupportedShellError is a shell this package cannot write.
type UnsupportedShellError struct {
	Shell string
}

func (unsupportedShellError *UnsupportedShellError) Error() string {
	return fmt.Sprintf(
		"the shell %q is not one this writes; it writes %s",
		unsupportedShellError.Shell,
		strings.Join(Shells, ", "),
	)
}

// parserProvider is implemented by a subparser that wraps a parser rather than being one.
//
// A program that needs to know which subcommand ran wraps each in a type of its own, and that type
// is opaque to this package unless it says what it holds. One that does not is still completed, by
// name and description; its options simply cannot be seen.
type parserProvider interface {
	GetParser() *argument_parser.Parser
}

// describer is implemented by a subparser that can describe itself.
type describer interface {
	GetDescription() string
}

// subcommand is one of a parser's commands, as this package needs it.
type subcommand struct {
	name        string
	description string
	// parser is nil where the subparser did not say what it holds.
	parser *argument_parser.Parser
}

// subcommands reads a parser's commands.
func subcommands(parser *argument_parser.Parser) []*subcommand {
	found := make([]*subcommand, 0, len(parser.Parsers))

	for _, subparser := range parser.Parsers {
		if subparser == nil {
			continue
		}

		name := subparser.GetCommand()
		if name == "" {
			continue
		}

		entry := &subcommand{name: name}

		if described, ok := subparser.(describer); ok {
			entry.description = described.GetDescription()
		}

		switch typed := subparser.(type) {
		case *argument_parser.Parser:
			entry.parser = typed
		case parserProvider:
			entry.parser = typed.GetParser()
		}

		found = append(found, entry)
	}

	return found
}

// takesArgument reports whether the option consumes a value.
func takesArgument(declared option.Option) bool {
	return declared.GetNargs() != option.NargsNone
}

// optionalArgument reports whether the option's value may be left out.
func optionalArgument(declared option.Option) bool {
	return declared.GetNargs() == option.NargsOptional
}

// repeatable reports whether the option may be given more than once.
//
// It matters to a completion because a shell hides an option once it has been used, and hiding one
// that accumulates would stop a caller giving the second of them.
func repeatable(declared option.Option) bool {
	return declared.GetNargs().IsVariadic()
}

// hidden reports whether the option is kept out of what is shown to a person.
//
// A hidden option is left out of the completion as well as the help. "Hidden" means one thing
// rather than two: an option a caller is not shown in the help is not one they should meet by
// pressing tab either, and an option marked hidden because it is deprecated or awkward would
// otherwise keep being offered.
func hidden(declared option.Option) bool {
	provider, ok := declared.(option.HiddenProvider)
	if !ok {
		return false
	}

	return provider.GetHidden()
}

// choices returns the values an option accepts, or nothing where it accepts any.
func choices(declared option.Option) []string {
	provider, ok := declared.(option.ChoicesProvider)
	if !ok {
		return nil
	}

	return provider.GetChoices()
}

// metavar is what to call an option's value.
func metavar(declared option.Option) string {
	if declared == nil {
		return ""
	}

	if given := declared.GetMetavar(); given != "" {
		return given
	}

	if long := declared.GetLongName(); long != "" {
		return strings.ToUpper(strings.ReplaceAll(long, "-", "_"))
	}

	return "ARG"
}

// summary is an option's usage as a shell can show it, which is one line.
//
// A usage string is written for the help, where it wraps over as many lines as it needs. A shell
// puts it in a column beside the option, so only the first sentence survives.
func summary(usage string) string {
	usage = strings.Join(strings.Fields(usage), " ")

	// The first sentence, where there is more than one. A full stop followed by a space ends one;
	// a full stop at the end of the string is the end of the only one.
	if index := strings.Index(usage, ". "); index >= 0 {
		usage = usage[:index+1]
	}

	return strings.TrimSuffix(usage, ".")
}

// exclusions maps each option to the others it rules out.
//
// This is the part a hand-written completion almost never has, because most parsers do not know it:
// having given --quiet, a caller should not be offered --verbose.
func exclusions(parser *argument_parser.Parser) map[option.Option][]option.Option {
	found := make(map[option.Option][]option.Option)

	for _, group := range parser.ExclusiveGroups {
		if group == nil {
			continue
		}

		for _, declared := range group.Options {
			if declared == nil {
				continue
			}

			for _, other := range group.Options {
				if other == nil || other == declared {
					continue
				}

				found[declared] = append(found[declared], other)
			}
		}
	}

	return found
}

// names returns an option's forms, short first, each with its dashes.
func names(declared option.Option) []string {
	found := make([]string, 0, 2)

	if short := declared.GetShortName(); short != "" {
		found = append(found, "-"+short)
	}
	if long := declared.GetLongName(); long != "" {
		found = append(found, "--"+long)
	}

	return found
}

// Write writes the completion for the parser to w.
func Write(writer io.Writer, parser *argument_parser.Parser, shell string) error {
	if writer == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("writer"))
	}

	if parser == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("parser"))
	}

	if shell == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("shell"))
	}

	if parser.ProgramName == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("program name"))
	}

	switch shell {
	case Zsh:
		return writeZsh(writer, parser)
	case Bash:
		return writeBash(writer, parser)
	default:
		return altshiftErrors.NewWithTrace(&UnsupportedShellError{Shell: shell})
	}
}

// Option is the option a program adds to offer completions of itself.
//
// It is opt-in rather than built into the parser: a program that never ships a completion should
// not carry the scripts to write one, and the shell the caller wants is a choice only they can
// make.
//
//	var shell string
//	parser.Options = append(parser.Options, completion.Option(&shell))
//	parser.ParseOrExit()
//	if shell != "" {
//		return completion.Write(os.Stdout, parser, shell)
//	}
//
// It is hidden: writing a completion is done once, when the program is installed, and an option
// that serves that has no business in the help of every invocation afterwards. It is accepted
// exactly as any other, and a program that would rather advertise it can declare its own.
func Option(shell *string) option.Option {
	return option.WithHidden(
		option.WithChoices(
			option.WithMetavar(
				option.NewStringOption(
					0,
					"completion",
					"Write a completion script for the given shell to standard output, and exit.",
					false,
					shell,
				),
				"SHELL",
			),
			Shells...,
		),
	)
}
