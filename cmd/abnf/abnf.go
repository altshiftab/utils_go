// Command abnf reads ABNF grammar definitions (RFC 5234, with the RFC 7405
// char-val extension and the RFC 9110 Section 5.6.1 "#" list operator): it
// reports on what they say at greater length than they need to, minifies
// them, and rewrites them in place.
package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	argumentParser "github.com/altshiftab/utils_go/pkg/cli/argument_parser"
	argumentParserErrors "github.com/altshiftab/utils_go/pkg/cli/argument_parser/errors"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	motmedelLog "github.com/altshiftab/utils_go/pkg/log"
	motmedelContextLogger "github.com/altshiftab/utils_go/pkg/log/context_logger"
	errorLogger "github.com/altshiftab/utils_go/pkg/log/error_logger"
)

// Exit codes. Anything the command was asked to report on that it found is
// told apart from the command failing to run at all.
const (
	exitClean    = 0
	exitReported = 1
	exitError    = 2
)

// stdinName stands in for the path of a definition read from standard input.
const stdinName = "stdin"

const description = "Read ABNF grammar definitions (RFC 5234, with the RFC 7405 char-val extension " +
	"and the RFC 9110 Section 5.6.1 \"#\" list operator)."

// command couples a subcommand's parser with what running it does, and
// records whether the command line named it. The parser is wrapped rather
// than used as a subparser directly so that the choice survives parsing.
type command struct {
	parser *argumentParser.Parser
	run    func() (int, error)
	chosen bool
}

func (command *command) GetCommand() string {
	return command.parser.GetCommand()
}

func (command *command) GetDescription() string {
	return command.parser.GetDescription()
}

func (command *command) ParseArgs(arguments []string) error {
	command.chosen = true

	if err := command.parser.ParseArgs(arguments); err != nil {
		return motmedelErrors.New(fmt.Errorf("parse args: %w", err), arguments)
	}

	return nil
}

func run() (int, error) {
	commands := []*command{newLintCommand(), newMinifyCommand(), newFixCommand()}

	subparsers := make([]argumentParser.Subparser, 0, len(commands))
	for _, command := range commands {
		command.parser.ProgramName = "abnf " + command.parser.Command
		subparsers = append(subparsers, command)
	}

	parser := &argumentParser.Parser{
		ProgramName: "abnf",
		Description: description,
		Parsers:     subparsers,
	}

	if err := parser.Validate(); err != nil {
		return exitError, motmedelErrors.New(fmt.Errorf("parser validate: %w", err))
	}

	if err := parser.Parse(); err != nil {
		// Help is an answer to an explicit request, not a failure.
		if errors.Is(err, argumentParserErrors.ErrHelp) {
			return exitClean, nil
		}

		fmt.Fprintf(os.Stderr, "abnf: %v\n", err)
		return exitError, nil
	}

	for _, command := range commands {
		if command.chosen {
			return command.run()
		}
	}

	// No subcommand was named.
	fmt.Fprint(os.Stderr, parser.FormatHelp())

	return exitError, nil
}

// readInput reads a definition from a path, or from standard input when the
// path is empty or a bare dash.
func readInput(path string) ([]byte, error) {
	if path == "" || path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, motmedelErrors.NewWithTrace(fmt.Errorf("io read all (stdin): %w", err))
		}
		return data, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, motmedelErrors.NewWithTrace(fmt.Errorf("os read file: %w", err), path)
	}

	return data, nil
}

// inputName returns what to call a definition read from the given path in a
// report.
func inputName(path string) string {
	if path == "" || path == "-" {
		return stdinName
	}
	return path
}

func main() {
	logger := errorLogger.Logger{
		Logger: motmedelContextLogger.New(
			slog.NewJSONHandler(os.Stderr, nil),
			&motmedelLog.ErrorContextExtractor{},
		),
	}
	slog.SetDefault(logger.Logger)

	code, err := run()
	if err != nil {
		logger.FatalWithExitingMessage("An error occurred.", err)
	}

	os.Exit(code)
}
