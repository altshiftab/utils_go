package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"os"
	"strings"

	"github.com/altshiftab/utils_go/pkg/abnf/lint"
	argumentParser "github.com/altshiftab/utils_go/pkg/cli/argument_parser"
	"github.com/altshiftab/utils_go/pkg/cli/argument_parser/option"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

const (
	formatText  = "text"
	formatSarif = "sarif"
)

// linted holds the findings of one definition together with the definition
// itself, which the text report quotes from.
type linted struct {
	report *lint.Report
	input  []byte
}

// newLintCommand makes the "lint" subcommand.
func newLintCommand() *command {
	var format string
	var simplify bool
	var roots []string
	var paths []string

	parser := &argumentParser.Parser{
		Command:     "lint",
		Description: "Report what a definition says at greater length than it needs to.",
		Options: []option.Option{
			option.WithDefault(
				option.WithChoices(
					option.NewStringOption('f', "format", "the format to report in", false, &format),
					formatText,
					formatSarif,
				),
				formatText,
			),
			option.NewBoolOption(
				's',
				"simplify",
				"also report expressions that can be written more shortly",
				false,
				&simplify,
			),
			// One rule per occurrence, accumulating: the default for a
			// strings option is greedy, which would swallow the paths that
			// follow it.
			option.WithNargs(
				option.NewStringsOption(
					'r',
					"root",
					"a rule the grammar is parsed from; given these, report the rules none of them reaches",
					false,
					&roots,
				),
				option.NargsOne,
			),
		},
		Rest: &paths,
	}

	return &command{
		parser: parser,
		run:    func() (int, error) { return runLint(format, simplify, roots, paths) },
	}
}

// runLint reports on every definition named, or on standard input when none
// is. It exits with exitReported when anything is reported, so that a check
// in a pipeline fails on a definition that is not as short as it could be.
func runLint(format string, simplify bool, roots []string, paths []string) (int, error) {
	if len(paths) == 0 {
		paths = []string{""}
	}

	options := &lint.Options{Simplify: simplify, Roots: roots}

	results := make([]*linted, 0, len(paths))
	for _, path := range paths {
		input, err := readInput(path)
		if err != nil {
			return exitError, altshiftErrors.New(fmt.Errorf("read input: %w", err))
		}

		findings, err := lint.Lint(input, options)
		if err != nil {
			return exitError, altshiftErrors.New(fmt.Errorf("lint: %w", err), inputName(path))
		}

		results = append(
			results,
			&linted{report: &lint.Report{Uri: inputName(path), Findings: findings}, input: input},
		)
	}

	if format == formatSarif {
		if err := writeSarif(results); err != nil {
			return exitError, altshiftErrors.New(fmt.Errorf("write sarif: %w", err))
		}
	} else {
		writeText(results)
	}

	for _, result := range results {
		if len(result.report.Findings) != 0 {
			return exitReported, nil
		}
	}

	return exitClean, nil
}

func writeSarif(results []*linted) error {
	reports := make([]*lint.Report, 0, len(results))
	for _, result := range results {
		reports = append(reports, result.report)
	}

	data, err := json.Marshal(lint.Sarif(reports), jsontext.WithIndent("  "))
	if err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("json marshal: %w", err))
	}

	if _, err := os.Stdout.Write(append(data, '\n')); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("os stdout write: %w", err))
	}

	return nil
}

// writeText reports findings the way golangci-lint does by default: the
// position, the message, the check in parentheses, and, for a finding
// pointing at something smaller than the line holding it, that line with a
// caret under it.
func writeText(results []*linted) {
	var sb strings.Builder

	total := 0
	for _, result := range results {
		lines := strings.Split(string(result.input), "\n")

		for _, finding := range result.report.Findings {
			total++

			fmt.Fprintf(
				&sb,
				"%s:%d:%d: %s (%s)\n",
				result.report.Uri,
				finding.Start.Line,
				finding.Start.Column,
				finding.Message,
				finding.RuleId,
			)

			line, ok := sourceLine(lines, finding.Start.Line)
			if !ok || !pointsWithinLine(finding, line) {
				continue
			}

			sb.WriteString(line)
			sb.WriteByte('\n')
			sb.WriteString(strings.Repeat(" ", max(finding.Start.Column-1, 0)))
			sb.WriteString("^\n")
		}
	}

	if total != 0 {
		fmt.Fprintf(&sb, "%d findings\n", total)
	}

	fmt.Fprint(os.Stdout, sb.String())
}

// sourceLine returns a one-based line of a definition, without its ending.
func sourceLine(lines []string, number int) (string, bool) {
	if number < 1 || number > len(lines) {
		return "", false
	}
	return strings.TrimRight(lines[number-1], "\r"), true
}

// pointsWithinLine reports whether a finding covers less than the whole line
// it starts on, which is when showing that line adds anything.
func pointsWithinLine(finding *lint.Finding, line string) bool {
	if finding.Start.Line != finding.End.Line {
		return false
	}
	return finding.End.Offset-finding.Start.Offset < len(strings.TrimRight(line, " \t"))
}
