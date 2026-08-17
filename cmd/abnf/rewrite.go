package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/altshiftab/utils_go/pkg/abnf/minify"
	argumentParser "github.com/altshiftab/utils_go/pkg/cli/argument_parser"
	"github.com/altshiftab/utils_go/pkg/cli/argument_parser/option"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// rewriteOptions are the options both rewriting subcommands take.
func rewriteOptions(simplify *bool, expand *bool) []option.Option {
	return []option.Option{
		option.NewBoolOption(
			's',
			"simplify",
			"also rewrite expressions into shorter equivalent ones, which changes the shape of parsed paths",
			false,
			simplify,
		),
		option.NewBoolOption(
			'e',
			"expand",
			"write the \"#\" list operators out as the standard ABNF they stand for, which is longer",
			false,
			expand,
		),
	}
}

// newMinifyCommand makes the "minify" subcommand.
func newMinifyCommand() *command {
	var outputPath string
	var simplify bool
	var expand bool
	var paths []string

	parser := &argumentParser.Parser{
		Command:     "minify",
		Description: "Write a definition without the whitespace and comments that carry nothing.",
		Options: append(
			rewriteOptions(&simplify, &expand),
			option.NewStringOption('o', "output", "the path to write to, rather than standard output", false, &outputPath),
		),
		Rest: &paths,
	}

	return &command{
		parser: parser,
		run:    func() (int, error) { return runMinify(outputPath, simplify, expand, paths) },
	}
}

// runMinify writes one definition, read from a path or from standard input,
// to a path or to standard output.
func runMinify(outputPath string, simplify bool, expand bool, paths []string) (int, error) {
	if len(paths) > 1 {
		fmt.Fprint(os.Stderr, "abnf minify: takes at most one path; use \"abnf fix\" for more than one\n")
		return exitError, nil
	}

	path := ""
	if len(paths) == 1 {
		path = paths[0]
	}

	input, err := readInput(path)
	if err != nil {
		return exitError, motmedelErrors.New(fmt.Errorf("read input: %w", err))
	}

	output, err := minify.Minify(input, &minify.Options{Simplify: simplify, ExpandLists: expand})
	if err != nil {
		return exitError, motmedelErrors.New(fmt.Errorf("minify: %w", err), inputName(path))
	}

	if outputPath == "" {
		if _, err := os.Stdout.Write(output); err != nil {
			return exitError, motmedelErrors.NewWithTrace(fmt.Errorf("os stdout write: %w", err))
		}
		return exitClean, nil
	}

	//nolint:gosec // G703: writing to the user-chosen output path is the command's purpose.
	if err := os.WriteFile(outputPath, output, 0600); err != nil {
		return exitError, motmedelErrors.NewWithTrace(fmt.Errorf("os write file: %w", err), outputPath)
	}

	return exitClean, nil
}

// newFixCommand makes the "fix" subcommand.
func newFixCommand() *command {
	var write bool
	var simplify bool
	var expand bool
	var paths []string

	parser := &argumentParser.Parser{
		Command:     "fix",
		Description: "Rewrite definitions in place, or name the ones that would change.",
		Options: append(
			rewriteOptions(&simplify, &expand),
			option.NewBoolOption(
				'w',
				"write",
				"rewrite the definitions in place, rather than naming the ones that would change",
				false,
				&write,
			),
		),
		Rest: &paths,
	}

	return &command{
		parser: parser,
		run:    func() (int, error) { return runFix(write, simplify, expand, paths) },
	}
}

// runFix rewrites definitions in place. Without -w it only names the ones
// that would change and exits with exitReported, as gofmt does, so that
// nothing is overwritten unless it was asked for.
func runFix(write bool, simplify bool, expand bool, paths []string) (int, error) {
	if len(paths) == 0 {
		fmt.Fprint(os.Stderr, "abnf fix: needs at least one path; use \"abnf minify\" to read standard input\n")
		return exitError, nil
	}

	options := &minify.Options{Simplify: simplify, ExpandLists: expand}

	changed := false
	for _, path := range paths {
		input, err := os.ReadFile(path)
		if err != nil {
			return exitError, motmedelErrors.NewWithTrace(fmt.Errorf("os read file: %w", err), path)
		}

		output, err := minify.Minify(input, options)
		if err != nil {
			return exitError, motmedelErrors.New(fmt.Errorf("minify: %w", err), path)
		}

		if bytes.Equal(output, input) {
			continue
		}
		changed = true

		if !write {
			fmt.Fprintln(os.Stdout, path)
			continue
		}

		// Keep the permissions the definition already had.
		mode := os.FileMode(0600)
		if info, err := os.Stat(path); err == nil {
			mode = info.Mode().Perm()
		}

		//nolint:gosec // G703: rewriting the definitions named on the command line is the command's purpose.
		if err := os.WriteFile(path, output, mode); err != nil {
			return exitError, motmedelErrors.NewWithTrace(fmt.Errorf("os write file: %w", err), path)
		}
	}

	if changed && !write {
		return exitReported, nil
	}

	return exitClean, nil
}
