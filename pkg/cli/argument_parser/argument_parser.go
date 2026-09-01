package argument_parser

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	argumentParserErrors "github.com/altshiftab/utils_go/pkg/cli/argument_parser/errors"
	"github.com/altshiftab/utils_go/pkg/cli/argument_parser/option"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

const (
	// defaultWidth is the assumed terminal width. The real width is only consulted through the
	// COLUMNS environment variable: reading it from the terminal itself needs an ioctl, and this
	// package does not take a dependency for that.
	defaultWidth = 80
	// maxHelpPosition caps how far a long option name may push the description column, matching
	// argparse's default. Terms wider than this get their description on the following line.
	maxHelpPosition = 24
	// termIndent is the indent of the term column in the help message.
	termIndent = "  "
	// dash is the argument that names standard input or output rather than an option.
	dash = "-"
)

// Subparser consumes the arguments that follow its command name.
type Subparser interface {
	ParseArgs([]string) error
	GetCommand() string
}

// describer is implemented by subparsers that can describe themselves in a parent's help message.
// *Parser implements it.
type describer interface {
	GetDescription() string
}

var _ Subparser = (*Parser)(nil)
var _ describer = (*Parser)(nil)

// Group titles a set of the parser's options in the help message. It only arranges the help; it
// does not affect what is accepted.
type Group struct {
	Title   string
	Options []option.Option
}

// ExclusiveGroup is a set of the parser's options of which at most one may be given.
type ExclusiveGroup struct {
	Options []option.Option
	// Required demands that one of them be given.
	Required bool
}

type Parser struct {
	Options []option.Option
	Parsers []Subparser
	// Groups arrange options under their own headings in the help message. An option that appears
	// in no group is listed under "Options:".
	Groups []*Group
	// ExclusiveGroups declare options that rule one another out.
	ExclusiveGroups []*ExclusiveGroup
	// Command is the name this parser answers to when it is used as a Subparser.
	Command     string
	Description string
	ProgramName string
	// Positionals are given the arguments that name no option, in order, according to how many
	// each takes. A positional is named in the help by its metavar.
	Positionals []option.Option
	// Rest receives the arguments left over once the positionals have taken theirs, including
	// everything after a "--" terminator. A leftover argument is an error while this is nil.
	Rest *[]string
	// DisableCompletion withholds the automatic --completion option, which a program that writes
	// its own completions or wants the name for something else turns off.
	DisableCompletion bool

	// DisableHelp withholds the automatic help option, leaving "-h" and "--help" to be used as
	// ordinary option names, or to be reported as unknown.
	DisableHelp bool
	// DisableAbbrev withholds prefix matching of long names. It is allowed by default, as argparse
	// and getopt_long allow it: an unambiguous prefix stands for the whole name, and an ambiguous
	// one is an error naming the candidates. Disabling it fixes the accepted command line to the
	// names as written, so that adding an option later cannot break a caller that spelled an
	// existing one short.
	DisableAbbrev bool
	// Output receives the help message. os.Stdout is used when nil: help is an answer to an
	// explicit request, not a diagnostic, so it must stay on stdout to survive a pipe.
	Output io.Writer
	// Width is the column at which the help message wraps. COLUMNS, then defaultWidth, is used
	// when this is not positive.
	Width int
}

// GetCommand returns the name this parser answers to as a subparser.
func (parser *Parser) GetCommand() string {
	return parser.Command
}

// GetDescription returns the description a parent parser shows beside this parser's command.
func (parser *Parser) GetDescription() string {
	return parser.Description
}

// Parse parses the arguments of the running program, excluding its name. An empty argument list is
// still parsed, so that a missing required option is reported.
func (parser *Parser) Parse() error {
	var arguments []string
	if osArgs := os.Args; len(osArgs) > 1 {
		arguments = osArgs[1:]
	}

	if err := parser.ParseArgs(arguments); err != nil {
		return altshiftErrors.New(fmt.Errorf("parse args: %w", err), arguments)
	}

	return nil
}

// lendTo gives a subparser about to run what it has no way to know: the command that reached it,
// and where and how wide its parent renders. Only what the subparser left unset is filled in, and
// only the fields whose zero value cannot be meant deliberately.
func (parser *Parser) lendTo(subparser Subparser, command string) {
	child, ok := subparser.(*Parser)
	if !ok {
		return
	}

	if child.ProgramName == "" {
		child.ProgramName = parser.getProgramName() + " " + command
	}
	if child.Output == nil {
		child.Output = parser.Output
	}
	if child.Width == 0 {
		child.Width = parser.Width
	}
}

// Validate reports what is wrong with the parser's own declaration, recursively through its
// subparsers, so that a duplicated option name surfaces at startup rather than on a first parse
// that happens to reach it.
func (parser *Parser) Validate() error {
	if _, err := makeNameTables(parser.Options); err != nil {
		return altshiftErrors.New(fmt.Errorf("make name tables: %w", err), parser.Options)
	}

	for _, opt := range parser.Options {
		if opt == nil {
			continue
		}

		normalizer, ok := opt.(option.ChoiceNormalizer)
		if !ok {
			continue
		}

		provider, ok := opt.(option.ChoicesProvider)
		if !ok {
			continue
		}

		for _, choice := range provider.GetChoices() {
			if _, err := normalizer.NormalizeChoice(choice); err != nil {
				return altshiftErrors.New(
					fmt.Errorf("normalize choice: %w", err),
					formatInvocation(opt),
					choice,
				)
			}
		}
	}

	if err := checkGroups(parser.Options, parser.Groups, parser.ExclusiveGroups); err != nil {
		return altshiftErrors.New(fmt.Errorf("check groups: %w", err), parser.Options)
	}

	if err := checkPositionals(parser.Positionals); err != nil {
		return altshiftErrors.New(fmt.Errorf("check positionals: %w", err), parser.Positionals)
	}

	for _, subparser := range parser.Parsers {
		if subparser == nil {
			continue
		}

		validator, ok := subparser.(interface{ Validate() error })
		if !ok {
			continue
		}

		if err := validator.Validate(); err != nil {
			return altshiftErrors.New(fmt.Errorf("subparser validate: %w", err), subparser)
		}
	}

	return nil
}

func (parser *Parser) getOutput() io.Writer {
	if output := parser.Output; output != nil {
		return output
	}

	return os.Stdout
}

func (parser *Parser) getWidth() int {
	if width := parser.Width; width > 0 {
		return width
	}

	if columns := os.Getenv("COLUMNS"); columns != "" {
		if width, err := strconv.Atoi(columns); err == nil && width > 0 {
			return width
		}
	}

	return defaultWidth
}

func (parser *Parser) getProgramName() string {
	if programName := parser.ProgramName; programName != "" {
		return programName
	}

	if len(os.Args) > 0 && os.Args[0] != "" {
		return filepath.Base(os.Args[0])
	}

	return "program"
}

// getHelpOption returns the automatic help option, bearing whichever of "h" and "help" no option
// of the parser's own has claimed, or nil when it is withheld or has no name left to answer to. It
// is an ordinary option so that it takes part in lookup, abbreviation and ambiguity like any
// other, which is how argparse and getopt_long treat theirs.
func (parser *Parser) getHelpOption() option.Option {
	if parser.DisableHelp {
		return nil
	}

	shortName := 'h'
	longName := "help"

	for _, opt := range parser.Options {
		if opt == nil {
			continue
		}

		if opt.GetShortName() == "h" {
			shortName = 0
		}
		if opt.GetLongName() == "help" {
			longName = ""
		}
	}

	if shortName == 0 && longName == "" {
		return nil
	}

	return option.NewBoolOption(
		shortName,
		longName,
		"Show this help message and exit",
		false,
		new(bool),
	)
}

// describe returns an option's usage text, noting the value it falls back on.
func describe(opt option.Option) string {
	description := opt.GetUsage()

	provider, ok := opt.(option.DefaultProvider)
	if !ok {
		return description
	}

	defaultValue := provider.GetDefault()
	if defaultValue == nil {
		return description
	}

	// Quote a default that would otherwise be invisible.
	shown := *defaultValue
	if shown == "" {
		shown = `""`
	}

	return strings.TrimSpace(description + " (default: " + shown + ")")
}

// getPositionalEntries returns the parser's positionals as the help message lists them, named by
// their metavar or by the choices they are restricted to.
func (parser *Parser) getPositionalEntries() []*entry {
	var entries []*entry

	for _, opt := range parser.Positionals {
		if opt == nil {
			continue
		}

		term := positionalName(opt)
		if provider, ok := opt.(option.ChoicesProvider); ok {
			if choices := provider.GetChoices(); len(choices) != 0 {
				term = "{" + strings.Join(choices, ",") + "}"
			}
		}

		entries = append(entries, &entry{term: term, description: describe(opt)})
	}

	return entries
}

// optionKey identifies an option by the names it answers to, which makeNameTables establishes are
// unique. Options are keyed this way rather than compared directly, so that an Option
// implementation need not be comparable.
func optionKey(opt option.Option) string {
	return opt.GetShortName() + "\x00" + opt.GetLongName()
}

// formatAlternation renders an exclusive group as the choice between its options that it is,
// bracketed when none of them need be given.
func formatAlternation(group *ExclusiveGroup) string {
	invocations := make([]string, 0, len(group.Options))
	for _, opt := range group.Options {
		if opt == nil {
			continue
		}

		invocations = append(invocations, formatInvocation(opt))
	}

	if len(invocations) == 0 {
		return ""
	}

	joined := strings.Join(invocations, " | ")
	if group.Required {
		return "(" + joined + ")"
	}

	return "[" + joined + "]"
}

// positionalName returns what the help and any complaint call a positional.
func positionalName(opt option.Option) string {
	if metavar := opt.GetMetavar(); metavar != "" {
		return metavar
	}

	return "ARGUMENT"
}

// formatPositional renders a positional as the usage line writes it, its brackets and ellipsis
// saying how many arguments it takes.
func formatPositional(opt option.Option) string {
	name := positionalName(opt)

	switch opt.GetNargs() {
	case option.NargsOptional:
		return "[" + name + "]"
	case option.NargsAtLeastOne:
		return name + "..."
	case option.NargsAny:
		return "[" + name + "...]"
	default:
		return name
	}
}

// checkPositionals reports a declaration whose positionals cannot be told apart. One variadic
// positional can be given whatever the fixed ones do not claim; a second has no such answer.
func checkPositionals(positionals []option.Option) error {
	variadic := 0

	for _, opt := range positionals {
		if opt == nil {
			continue
		}

		if opt.GetNargs().IsVariadic() {
			variadic++
		}
	}

	if variadic > 1 {
		return altshiftErrors.NewWithTrace(argumentParserErrors.ErrAmbiguousPositionals)
	}

	return nil
}

// assignPositionals hands the arguments that named no option to the positionals in turn, each
// taking as many as it may without starving those after it, and returns whatever is left over.
func assignPositionals(positionals []option.Option, arguments []string) ([]string, error) {
	index := 0

	for position, opt := range positionals {
		if opt == nil {
			continue
		}

		// What the positionals after this one must still be given, which this one may not take.
		reserved := 0
		for _, later := range positionals[position+1:] {
			if later != nil {
				reserved += later.GetNargs().Minimum()
			}
		}

		nargs := opt.GetNargs()
		available := len(arguments) - index - reserved

		var count int
		switch {
		case nargs.IsVariadic():
			count = max(available, 0)
		case nargs == option.NargsOptional:
			if available > 0 {
				count = 1
			}
		default:
			count = 1
		}

		if count < nargs.Minimum() || index+count > len(arguments) {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf(
					"%w: %s",
					argumentParserErrors.ErrMissingPositional,
					positionalName(opt),
				),
			)
		}

		if count == 0 {
			// A positional that was given nothing falls back to its default, as an option does.
			if provider, ok := opt.(option.DefaultProvider); ok {
				if defaultValue := provider.GetDefault(); defaultValue != nil {
					if err := setOption(opt, *defaultValue); err != nil {
						return nil, err
					}
				}
			}

			continue
		}

		for range count {
			if err := setOption(opt, arguments[index]); err != nil {
				return nil, err
			}

			index++
		}
	}

	if index >= len(arguments) {
		return nil, nil
	}

	return arguments[index:], nil
}

// checkGroups reports an option that a group names but the parser does not declare. Such an option
// appears in the help and can never be given, which is a mistake in the declaration rather than in
// the command line.
func checkGroups(options []option.Option, groups []*Group, exclusiveGroups []*ExclusiveGroup) error {
	declared := make(map[string]struct{}, len(options))
	for _, opt := range options {
		if opt != nil {
			declared[optionKey(opt)] = struct{}{}
		}
	}

	checkMember := func(opt option.Option) error {
		if opt == nil {
			return nil
		}

		if _, ok := declared[optionKey(opt)]; ok {
			return nil
		}

		return altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %s", argumentParserErrors.ErrUndeclaredOption, formatInvocation(opt)),
		)
	}

	for _, group := range groups {
		if group == nil {
			continue
		}

		for _, opt := range group.Options {
			if err := checkMember(opt); err != nil {
				return err
			}
		}
	}

	for _, group := range exclusiveGroups {
		if group == nil {
			continue
		}

		for _, opt := range group.Options {
			if err := checkMember(opt); err != nil {
				return err
			}
		}
	}

	return nil
}

// checkExclusive reports options that rule one another out but were given together, and groups
// that demanded one of their options and got none.
func checkExclusive(groups []*ExclusiveGroup, seen *seenNames) error {
	for _, group := range groups {
		if group == nil {
			continue
		}

		var given []string
		var all []string

		for _, opt := range group.Options {
			if opt == nil {
				continue
			}

			invocation := formatInvocation(opt)
			all = append(all, invocation)
			if seen.has(opt.GetShortName(), opt.GetLongName()) {
				given = append(given, invocation)
			}
		}

		if len(given) > 1 {
			return altshiftErrors.NewWithTrace(
				fmt.Errorf(
					"%w: %s",
					argumentParserErrors.ErrMutuallyExclusiveOptions,
					strings.Join(given, ", "),
				),
			)
		}

		if group.Required && len(given) == 0 && len(all) != 0 {
			return altshiftErrors.NewWithTrace(
				fmt.Errorf(
					"%w: one of %s",
					argumentParserErrors.ErrMissingRequiredOption,
					strings.Join(all, ", "),
				),
			)
		}
	}

	return nil
}

// entry is one term of a help message and the text describing it.
type entry struct {
	term        string
	description string
}

// getCommandEntries returns the parser's subcommands and what they describe themselves as.
func (parser *Parser) getCommandEntries() []*entry {
	var entries []*entry

	for _, subparser := range parser.Parsers {
		if subparser == nil {
			continue
		}

		command := subparser.GetCommand()
		if command == "" {
			continue
		}

		var description string
		if subparserDescriber, ok := subparser.(describer); ok {
			description = subparserDescriber.GetDescription()
		}

		entries = append(entries, &entry{term: command, description: description})
	}

	return entries
}

// isHidden reports whether the option is kept out of what is shown to a person.
//
// An option that does not say is shown, so that an Option written by hand rather than by the option
// package does not have to know about this at all.
func isHidden(opt option.Option) bool {
	provider, ok := opt.(option.HiddenProvider)
	if !ok {
		return false
	}

	return provider.GetHidden()
}

// getOptionEntries returns the parser's options as they are written in the help message. Only the
// options belonging to the named group are returned; passing the empty title returns those in no
// group, along with the automatic help option.
func (parser *Parser) getOptionEntries(title string) []*entry {
	var options []option.Option

	if title == "" {
		grouped := make(map[string]struct{})
		for _, group := range parser.Groups {
			if group == nil {
				continue
			}

			for _, opt := range group.Options {
				if opt != nil {
					grouped[optionKey(opt)] = struct{}{}
				}
			}
		}

		for _, opt := range parser.Options {
			if opt == nil || isHidden(opt) {
				continue
			}

			if _, ok := grouped[optionKey(opt)]; !ok {
				options = append(options, opt)
			}
		}

		if helpOption := parser.getHelpOption(); helpOption != nil {
			options = append(options, helpOption)
		}
	} else {
		for _, group := range parser.Groups {
			if group == nil || group.Title != title {
				continue
			}

			for _, opt := range group.Options {
				if opt != nil && !isHidden(opt) {
					options = append(options, opt)
				}
			}
		}
	}

	entries := make([]*entry, 0, len(options))

	for _, opt := range options {
		if opt == nil {
			continue
		}

		var term strings.Builder
		shortName := opt.GetShortName()
		longName := opt.GetLongName()

		if shortName != "" {
			term.WriteString("-")
			term.WriteString(shortName)
		}

		if shortName != "" && longName != "" {
			term.WriteString(", ")
		}

		if longName != "" {
			if shortName == "" {
				term.WriteString("    ")
			}
			term.WriteString("--")
			term.WriteString(longName)
		}

		if metavar := formatMetavar(opt); metavar != "" {
			term.WriteString(" ")
			term.WriteString(metavar)
		}

		entries = append(entries, &entry{term: term.String(), description: describe(opt)})
	}

	return entries
}

// formatMetavar renders how an option's argument is written: the choices it is restricted to, or
// its metavar, marked for repetition or optionality.
func formatMetavar(opt option.Option) string {
	nargs := opt.GetNargs()
	if nargs == option.NargsNone {
		return ""
	}

	text := opt.GetMetavar()
	if provider, ok := opt.(option.ChoicesProvider); ok {
		if choices := provider.GetChoices(); len(choices) != 0 {
			text = "{" + strings.Join(choices, ",") + "}"
		}
	}

	if text == "" {
		return ""
	}

	switch nargs {
	case option.NargsAtLeastOne:
		return text + "..."
	case option.NargsAny:
		return "[" + text + "...]"
	case option.NargsOptional:
		return "[" + text + "]"
	default:
		return text
	}
}

// formatInvocation renders how an option is written on the command line, preferring its short
// name, as "-i INT" or "--only-long STRING".
func formatInvocation(opt option.Option) string {
	var builder strings.Builder

	if shortName := opt.GetShortName(); shortName != "" {
		builder.WriteString("-")
		builder.WriteString(shortName)
	} else {
		builder.WriteString("--")
		builder.WriteString(opt.GetLongName())
	}

	if metavar := formatMetavar(opt); metavar != "" {
		builder.WriteString(" ")
		builder.WriteString(metavar)
	}

	return builder.String()
}

// wrapText breaks text into lines of at most width columns, collapsing runs of whitespace.
func wrapText(text string, width int) []string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return nil
	}

	if width <= 0 {
		return []string{strings.Join(fields, " ")}
	}

	var lines []string
	var line strings.Builder

	for _, field := range fields {
		switch {
		case line.Len() == 0:
			line.WriteString(field)
		case line.Len()+1+len(field) > width:
			lines = append(lines, line.String())
			line.Reset()
			line.WriteString(field)
		default:
			line.WriteString(" ")
			line.WriteString(field)
		}
	}

	if line.Len() != 0 {
		lines = append(lines, line.String())
	}

	return lines
}

// getHelpColumn returns the column at which descriptions begin: clear of the terms, but never
// past maxHelpPosition.
func getHelpColumn(entryGroups ...[]*entry) int {
	column := 0

	for _, entries := range entryGroups {
		for _, helpEntry := range entries {
			if candidate := len(termIndent) + len(helpEntry.term) + 2; candidate > column {
				column = candidate
			}
		}
	}

	return min(column, maxHelpPosition)
}

// writeEntries renders entries as a two-column list, moving a description to the following line
// when its term reaches into the description column.
func writeEntries(builder *strings.Builder, entries []*entry, column int, width int) {
	for _, helpEntry := range entries {
		lines := wrapText(helpEntry.description, width-column)

		builder.WriteString(termIndent)
		builder.WriteString(helpEntry.term)

		if len(lines) == 0 {
			builder.WriteString("\n")
			continue
		}

		used := len(termIndent) + len(helpEntry.term)
		if used+2 > column {
			builder.WriteString("\n")
			used = 0
		}

		for index, line := range lines {
			if index == 0 && used != 0 {
				builder.WriteString(strings.Repeat(" ", column-used))
			} else {
				builder.WriteString(strings.Repeat(" ", column))
			}
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}
}

// formatUsage renders the usage line, enumerating the options as they may be written. Optional
// options are bracketed; required ones are not.
func (parser *Parser) formatUsage(width int) string {
	prefix := "Usage: " + parser.getProgramName()

	var parts []string

	if helpOption := parser.getHelpOption(); helpOption != nil {
		parts = append(parts, "["+formatInvocation(helpOption)+"]")
	}

	keyToExclusive := make(map[string]int)
	for index, group := range parser.ExclusiveGroups {
		if group == nil {
			continue
		}

		for _, opt := range group.Options {
			if opt != nil {
				keyToExclusive[optionKey(opt)] = index
			}
		}
	}

	emitted := make(map[int]struct{})

	for _, opt := range parser.Options {
		if opt == nil || isHidden(opt) {
			continue
		}

		// An option that rules others out is written as the choice between them, once, where the
		// first of them would have gone.
		if index, ok := keyToExclusive[optionKey(opt)]; ok {
			if _, done := emitted[index]; done {
				continue
			}
			emitted[index] = struct{}{}

			if alternation := formatAlternation(parser.ExclusiveGroups[index]); alternation != "" {
				parts = append(parts, alternation)
			}

			continue
		}

		if opt.GetRequired() {
			parts = append(parts, formatInvocation(opt))
		} else {
			parts = append(parts, "["+formatInvocation(opt)+"]")
		}
	}

	for _, opt := range parser.Positionals {
		if opt != nil {
			parts = append(parts, formatPositional(opt))
		}
	}

	if entries := parser.getCommandEntries(); len(entries) > 0 {
		commands := make([]string, 0, len(entries))
		for _, commandEntry := range entries {
			commands = append(commands, commandEntry.term)
		}

		parts = append(parts, "{"+strings.Join(commands, ",")+"}", "...")
	}

	if parser.Rest != nil {
		parts = append(parts, "[--]", "[ARGUMENT...]")
	}

	var builder strings.Builder
	builder.WriteString(prefix)

	indent := strings.Repeat(" ", len(prefix)+1)
	lineLength := len(prefix)

	for _, part := range parts {
		if lineLength+1+len(part) > width && lineLength > len(indent) {
			builder.WriteString("\n")
			builder.WriteString(indent)
			builder.WriteString(part)
			lineLength = len(indent) + len(part)

			continue
		}

		builder.WriteString(" ")
		builder.WriteString(part)
		lineLength += 1 + len(part)
	}

	return builder.String()
}

// FormatUsage returns the usage line on its own. A report of a bad invocation conventionally
// begins with it, so that being told what went wrong also says how the program is meant to be run.
func (parser *Parser) FormatUsage() string {
	return parser.formatUsage(parser.getWidth())
}

// FormatError renders err the way a command-line program conventionally reports a bad invocation:
// the usage line, then the program name and what went wrong.
func (parser *Parser) FormatError(err error) string {
	var builder strings.Builder

	builder.WriteString(parser.FormatUsage())
	builder.WriteString("\n")
	builder.WriteString(parser.getProgramName())
	builder.WriteString(": error: ")
	if err != nil {
		builder.WriteString(err.Error())
	}
	builder.WriteString("\n")

	return builder.String()
}

// ParseOrExit parses the running program's arguments and returns only if that succeeded. A request
// for help has already been answered on stdout, and leaves through status 0; a bad invocation is
// reported on stderr and leaves through status 2, as a command-line program conventionally does.
func (parser *Parser) ParseOrExit() {
	message, code, ok := parser.report(parser.Parse())
	if message != "" {
		fmt.Fprint(os.Stderr, message)
	}

	if !ok {
		os.Exit(code)
	}
}

// report decides what a parse outcome means for a command-line program: what to write to stderr,
// the status to leave through, and whether to carry on at all. It is what ParseOrExit does, minus
// the exiting.
func (parser *Parser) report(err error) (string, int, bool) {
	switch {
	case err == nil:
		return "", 0, true
	case errors.Is(err, argumentParserErrors.ErrHelp), errors.Is(err, argumentParserErrors.ErrCompletion):
		// Already answered on stdout; there is nothing left to say or do.
		return "", 0, false
	default:
		return parser.FormatError(err), 2, false
	}
}

// FormatHelp returns the usage message describing the parser's commands and options.
func (parser *Parser) FormatHelp() string {
	width := parser.getWidth()

	var builder strings.Builder

	builder.WriteString(parser.formatUsage(width))
	builder.WriteString("\n")

	if description := parser.Description; description != "" {
		builder.WriteString("\n")
		for _, line := range wrapText(description, width) {
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}

	commandEntries := parser.getCommandEntries()
	positionalEntries := parser.getPositionalEntries()
	optionEntries := parser.getOptionEntries("")

	// Every section shares one description column, so the help reads as a single list however it
	// is divided up.
	entryGroups := [][]*entry{commandEntries, positionalEntries, optionEntries}

	type section struct {
		title   string
		entries []*entry
	}

	var sections []*section
	for _, group := range parser.Groups {
		if group == nil {
			continue
		}

		groupEntries := parser.getOptionEntries(group.Title)
		if len(groupEntries) == 0 {
			continue
		}

		sections = append(sections, &section{title: group.Title, entries: groupEntries})
		entryGroups = append(entryGroups, groupEntries)
	}

	column := getHelpColumn(entryGroups...)

	if len(commandEntries) != 0 {
		builder.WriteString("\nCommands:\n")
		writeEntries(&builder, commandEntries, column, width)
	}

	if len(positionalEntries) != 0 {
		builder.WriteString("\nArguments:\n")
		writeEntries(&builder, positionalEntries, column, width)
	}

	if len(optionEntries) != 0 {
		builder.WriteString("\nOptions:\n")
		writeEntries(&builder, optionEntries, column, width)
	}

	for _, helpSection := range sections {
		builder.WriteString("\n")
		builder.WriteString(helpSection.title)
		builder.WriteString(":\n")
		writeEntries(&builder, helpSection.entries, column, width)
	}

	return builder.String()
}

// parsedArgument is the option syntax of a single argument.
type parsedArgument struct {
	// names is the long name of a "--name" argument, or each of the clustered short names of a
	// "-abc" argument.
	names []string
	// inlineValue is the value of a "--name=value" argument. It is non-nil even when empty.
	inlineValue *string
	// long records which form was written, so that a long name is reachable only with two dashes
	// and a short name only with one.
	long bool
}

// parseArgument returns how an argument names options, or nil when it names none.
func parseArgument(argument string) *parsedArgument {
	if argument == "" {
		return nil
	}

	// A lone dash is an operand, not an option: by long convention it names standard input or output,
	// and both getopt and argparse hand it to the program as a positional. Read as an option it is a
	// cluster of no names at all, which is silently dropped on the way past.
	if argument == dash {
		return nil
	}

	if before, longName, found := strings.Cut(argument, "--"); found && before == "" {
		name, inlineValue, found := strings.Cut(longName, "=")
		if !found {
			return &parsedArgument{names: []string{longName}, long: true}
		}
		if name == "" {
			return nil
		}

		return &parsedArgument{names: []string{name}, inlineValue: &inlineValue, long: true}
	} else if before, shortNames, found := strings.Cut(argument, "-"); found && before == "" {
		return &parsedArgument{names: strings.Split(shortNames, "")}
	}

	return nil
}

// isNegativeNumber reports whether an argument reads as a negative number rather than an option,
// using the same shape as argparse: a leading "-" followed by digits, optionally with a single
// decimal point.
func isNegativeNumber(argument string) bool {
	rest, found := strings.CutPrefix(argument, "-")
	if !found || rest == "" {
		return false
	}

	integerPart, fractionPart, hasFraction := strings.Cut(rest, ".")
	if !hasFraction {
		return integerPart != "" && isDigits(integerPart)
	}

	return isDigits(integerPart) && fractionPart != "" && isDigits(fractionPart)
}

func isDigits(text string) bool {
	for _, character := range text {
		if character < '0' || character > '9' {
			return false
		}
	}

	return true
}

// nameTables resolves option names. Short and long names are kept apart so that a short name is
// reachable only with one dash and a long name only with two, and so that an option may use the
// same text for both.
type nameTables struct {
	short map[string]option.Option
	long  map[string]option.Option
}

func makeNameTables(options []option.Option) (*nameTables, error) {
	tables := &nameTables{
		short: make(map[string]option.Option),
		long:  make(map[string]option.Option),
	}

	for _, opt := range options {
		if opt == nil {
			continue
		}

		for _, pair := range []struct {
			name  string
			table map[string]option.Option
		}{
			{name: opt.GetShortName(), table: tables.short},
			{name: opt.GetLongName(), table: tables.long},
		} {
			if pair.name == "" {
				continue
			}

			if _, ok := pair.table[pair.name]; ok {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf(
						"%w: %s",
						argumentParserErrors.ErrMultipleOptionsWithSameName,
						pair.name,
					),
				)
			}

			pair.table[pair.name] = opt
		}
	}

	return tables, nil
}

func (tables *nameTables) lookup(name string, long bool) (option.Option, bool) {
	table := tables.short
	if long {
		table = tables.long
	}

	opt, ok := table[name]
	if !ok || opt == nil {
		return nil, false
	}

	return opt, true
}

// hasNegativeNumberName reports whether any short name makes an argument like "-1" an option.
func (tables *nameTables) hasNegativeNumberName() bool {
	for name := range tables.short {
		if isNegativeNumber("-" + name) {
			return true
		}
	}

	return false
}

// seenNames records the names that arguments used, kept apart by form for the same reason the
// lookup tables are. Names are recorded rather than options themselves so that an Option
// implementation need not be comparable.
type seenNames struct {
	short map[string]struct{}
	long  map[string]struct{}
}

func makeSeenNames() *seenNames {
	return &seenNames{short: make(map[string]struct{}), long: make(map[string]struct{})}
}

func (seen *seenNames) record(name string, long bool) {
	if long {
		seen.long[name] = struct{}{}
		return
	}

	seen.short[name] = struct{}{}
}

func (seen *seenNames) has(shortName string, longName string) bool {
	if shortName != "" {
		if _, ok := seen.short[shortName]; ok {
			return true
		}
	}

	if longName != "" {
		if _, ok := seen.long[longName]; ok {
			return true
		}
	}

	return false
}

// resolveAbbreviation returns the long option a prefix names, along with every long name the
// prefix matches. The names are sorted so that an ambiguity reports the same way each time.
func (tables *nameTables) resolveAbbreviation(prefix string) (option.Option, []string) {
	var opt option.Option
	var matches []string

	for name, candidate := range tables.long {
		if strings.HasPrefix(name, prefix) {
			matches = append(matches, name)
			opt = candidate
		}
	}

	slices.Sort(matches)

	return opt, matches
}

// normalizeChoice renders a value the way the option spells its own, so that "007" and "7" are the
// same choice of an int option. A value the option cannot read is returned unchanged, so it
// matches no choice and is reported as one that was not offered — which tells the caller more than
// a conversion error would.
func normalizeChoice(opt option.Option, value string) string {
	normalizer, ok := opt.(option.ChoiceNormalizer)
	if !ok {
		return value
	}

	normalized, err := normalizer.NormalizeChoice(value)
	if err != nil {
		return value
	}

	return normalized
}

// offersChoice reports whether a value is among an option's choices, comparing both in the
// option's normal form.
func offersChoice(opt option.Option, choices []string, value string) bool {
	normalized := normalizeChoice(opt, value)

	for _, choice := range choices {
		if normalizeChoice(opt, choice) == normalized {
			return true
		}
	}

	return false
}

// setOption gives an option a value, refusing one the option does not offer.
func setOption(opt option.Option, value string) error {
	if provider, ok := opt.(option.ChoicesProvider); ok {
		if choices := provider.GetChoices(); len(choices) != 0 && !offersChoice(opt, choices, value) {
			return altshiftErrors.NewWithTrace(
				fmt.Errorf(
					"%w: %s (choose from %s)",
					argumentParserErrors.ErrInvalidChoice,
					value,
					strings.Join(choices, ", "),
				),
				value,
			)
		}
	}

	if err := opt.Set(value); err != nil {
		return altshiftErrors.New(fmt.Errorf("option set: %w", err), opt, value)
	}

	return nil
}

// closePending settles an option that is still awaiting a value. One whose argument is optional
// falls back to its constant; any other was left unset.
func closePending(pending option.Option, pendingName string, pendingCount int) error {
	if pending == nil || pendingCount != 0 {
		return nil
	}

	nargs := pending.GetNargs()
	if nargs == option.NargsAny {
		// Any number includes none.
		return nil
	}

	if nargs != option.NargsOptional {
		return altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %s", argumentParserErrors.ErrUnsetOption, pendingName),
		)
	}

	var constValue string
	if provider, ok := pending.(option.ConstProvider); ok {
		constValue = provider.GetConst()
	}

	return setOption(pending, constValue)
}

// applyDefaults gives each option that no argument named its declared default.
func applyDefaults(options []option.Option, seen *seenNames) error {
	for _, opt := range options {
		if opt == nil {
			continue
		}

		provider, ok := opt.(option.DefaultProvider)
		if !ok {
			continue
		}

		defaultValue := provider.GetDefault()
		if defaultValue == nil || seen.has(opt.GetShortName(), opt.GetLongName()) {
			continue
		}

		if err := setOption(opt, *defaultValue); err != nil {
			return err
		}
	}

	return nil
}

// checkRequired reports the first required option that no argument named.
func checkRequired(options []option.Option, seen *seenNames) error {
	for _, opt := range options {
		if opt == nil || !opt.GetRequired() {
			continue
		}

		if seen.has(opt.GetShortName(), opt.GetLongName()) {
			continue
		}

		return altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: %s",
				argumentParserErrors.ErrMissingRequiredOption,
				formatInvocation(opt),
			),
		)
	}

	return nil
}

// ParseArgs parses arguments, dispatching to a subparser when the first argument names one.
func (parser *Parser) ParseArgs(arguments []string) error {
	if parsers := parser.Parsers; len(parsers) != 0 && len(arguments) != 0 {
		firstArgument := arguments[0]

		for _, subparser := range parsers {
			if subparser == nil {
				continue
			}

			if subparser.GetCommand() == firstArgument {
				parser.lendTo(subparser, firstArgument)

				subcommandArguments := arguments[1:]
				if err := subparser.ParseArgs(subcommandArguments); err != nil {
					return altshiftErrors.New(
						fmt.Errorf("subcommand parse args: %w", err),
						subparser,
						subcommandArguments,
					)
				}

				return nil
			}
		}
	}

	options := parser.Options

	// The help option joins the lookup tables rather than being matched ahead of them, so that a
	// prefix may reach it and an option sharing that prefix makes it ambiguous, as elsewhere.
	tableOptions := options
	helpOption := parser.getHelpOption()
	if helpOption != nil {
		tableOptions = append(slices.Clone(options), helpOption)
	}

	// The completion option joins them the same way. Unlike help it takes a value, so it cannot be
	// answered the moment it is matched: the shell it was given has to be read first.
	var completionShell string
	completionOption := parser.getCompletionOption(&completionShell)
	if completionOption != nil {
		tableOptions = append(slices.Clone(tableOptions), completionOption)
	}

	tables, err := makeNameTables(tableOptions)
	if err != nil {
		return altshiftErrors.New(fmt.Errorf("make name tables: %w", err), options)
	}

	if err := checkGroups(options, parser.Groups, parser.ExclusiveGroups); err != nil {
		return altshiftErrors.New(fmt.Errorf("check groups: %w", err), options)
	}

	if err := checkPositionals(parser.Positionals); err != nil {
		return altshiftErrors.New(fmt.Errorf("check positionals: %w", err), parser.Positionals)
	}

	// An option named like a negative number makes "-1" ambiguous; argparse resolves it in favour
	// of the option, and so does this.
	hasNegativeNumberName := tables.hasNegativeNumberName()

	// pending is the option awaiting a value, cleared as soon as it has all it takes. pendingCount
	// counts the values it has been given, so that an option left without one is reported.
	var pending option.Option
	var pendingName string
	var pendingCount int

	seen := makeSeenNames()
	var rest []string

	for index, argument := range arguments {
		if argument == "--" {
			if err := closePending(pending, pendingName, pendingCount); err != nil {
				return err
			}

			pending = nil
			rest = append(rest, arguments[index+1:]...)

			break
		}

		parsed := parseArgument(argument)
		if parsed != nil && !parsed.long && !hasNegativeNumberName && isNegativeNumber(argument) {
			parsed = nil
		}

		if parsed == nil {
			if pending == nil {
				rest = append(rest, argument)

				continue
			}

			if err := setOption(pending, argument); err != nil {
				return err
			}

			pendingCount++
			if !pending.GetNargs().IsVariadic() {
				pending = nil
			}

			continue
		}

	cluster:
		for index, name := range parsed.names {
			opt, ok := tables.lookup(name, parsed.long)
			if !ok && parsed.long && !parser.DisableAbbrev {
				abbreviated, matches := tables.resolveAbbreviation(name)
				if len(matches) > 1 {
					return altshiftErrors.NewWithTrace(
						fmt.Errorf(
							"%w: %s could match %s",
							argumentParserErrors.ErrAmbiguousOption,
							name,
							strings.Join(matches, ", "),
						),
					)
				}
				if len(matches) == 1 && abbreviated != nil {
					opt, ok = abbreviated, true
					name = matches[0]
				}
			}
			if !ok {
				return altshiftErrors.NewWithTrace(
					fmt.Errorf("%w: %s", argumentParserErrors.ErrNameNotFound, name),
				)
			}

			if opt == helpOption {
				fmt.Fprint(parser.getOutput(), parser.FormatHelp())
				return argumentParserErrors.ErrHelp
			}

			if err := closePending(pending, pendingName, pendingCount); err != nil {
				return err
			}
			pending = nil

			seen.record(name, parsed.long)

			nargs := opt.GetNargs()

			// A short option that takes a value ends its cluster: whatever follows it in the same
			// argument is the value, however much of it looks like further option names. This is
			// the getopt convention, by which "-n5" and "-abn5" mean "-n 5" and "-a -b -n 5".
			var attachedValue *string
			if !parsed.long && nargs != option.NargsNone && index+1 < len(parsed.names) {
				value := strings.Join(parsed.names[index+1:], "")
				attachedValue = &value
			}

			switch {
			case parsed.inlineValue != nil:
				if nargs == option.NargsNone {
					return altshiftErrors.NewWithTrace(
						fmt.Errorf(
							"%w: %s=%s",
							argumentParserErrors.ErrUnexpectedOptionValue,
							name,
							*parsed.inlineValue,
						),
					)
				}

				if err := setOption(opt, *parsed.inlineValue); err != nil {
					return err
				}
			case attachedValue != nil:
				if err := setOption(opt, *attachedValue); err != nil {
					return err
				}

				break cluster
			case nargs == option.NargsNone:
				if err := setOption(opt, ""); err != nil {
					return err
				}
			default:
				pending = opt
				pendingName = name
				pendingCount = 0
			}
		}
	}

	if err := closePending(pending, pendingName, pendingCount); err != nil {
		return err
	}

	// Answered before the positionals are assigned and before anything required is insisted on,
	// because writing a completion is not a run of the program: a caller asking for one has no
	// reason to satisfy arguments they are not using.
	if completionOption != nil && completionShell != "" {
		if err := parser.WriteCompletion(parser.getOutput(), completionShell); err != nil {
			return fmt.Errorf("write completion: %w", err)
		}

		return argumentParserErrors.ErrCompletion
	}

	leftover, err := assignPositionals(parser.Positionals, rest)
	if err != nil {
		return altshiftErrors.New(fmt.Errorf("assign positionals: %w", err), rest)
	}

	if len(leftover) != 0 && parser.Rest == nil {
		return altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %s", argumentParserErrors.ErrUnexpectedArgument, leftover[0]),
		)
	}

	if parser.Rest != nil {
		*parser.Rest = leftover
	}

	if err := checkRequired(options, seen); err != nil {
		return err
	}

	if err := checkExclusive(parser.ExclusiveGroups, seen); err != nil {
		return err
	}

	return applyDefaults(options, seen)
}
