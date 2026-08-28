package completion

import (
	"fmt"
	"io"
	"strings"

	"github.com/altshiftab/utils_go/pkg/cli/argument_parser"
	"github.com/altshiftab/utils_go/pkg/cli/argument_parser/option"
)

// zshQuote wraps a string in single quotes, which is the only zsh quoting that means what it says.
//
// A single quote inside cannot be escaped within single quotes, so the string is closed, an escaped
// quote is written, and it is opened again -- the '\” that shell scripts are full of.
func zshQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// zshDescription escapes a description for the brackets of an _arguments spec.
//
// A bracket inside would end the description early and leave the rest of it read as part of the
// spec, which is how a stray "[" in a usage string turns into a broken completion.
func zshDescription(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`, ":", `\:`)

	return replacer.Replace(value)
}

// zshIdentifier is the name of a generated function, which may hold only what a shell allows.
func zshIdentifier(value string) string {
	var builder strings.Builder

	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z',
			character >= 'A' && character <= 'Z',
			character >= '0' && character <= '9',
			character == '_':
			builder.WriteRune(character)
		default:
			builder.WriteRune('_')
		}
	}

	return builder.String()
}

// zshValueSpec is the part of a spec that says what an option's value may be.
func zshValueSpec(declared option.Option) string {
	if !takesArgument(declared) {
		return ""
	}

	name := zshDescription(metavar(declared))

	// The values the option accepts, where it accepts only some. The parser validates against the
	// same list, so the completion cannot offer something that would then be rejected.
	if accepted := choices(declared); len(accepted) != 0 {
		escaped := make([]string, 0, len(accepted))
		for _, choice := range accepted {
			escaped = append(escaped, zshDescription(choice))
		}

		return ":" + name + ":(" + strings.Join(escaped, " ") + ")"
	}

	// A metavar naming a path is completed as one. It is a guess, but a cheap and usually right
	// one, and a program that wants otherwise names the value something else.
	switch strings.ToUpper(metavar(declared)) {
	case "PATH", "FILE":
		return ":" + name + ":_files"
	case "DIR", "DIRECTORY":
		return ":" + name + ":_directories"
	default:
		return ":" + name + ":"
	}
}

// zshOptionSpec renders one option as an _arguments spec, quoted ready for the shell.
//
// The quoting is the subtle part, and getting it wrong produces a script that parses but does not
// work. Where an option has both a short and a long form, the {-b,--brute} brace expansion has to
// reach the shell unquoted, so that it expands into two arguments -- one spec per form. Quoting the
// whole spec as one string instead leaves the braces literal, and _arguments rejects it with
// "invalid argument" at completion time, which no syntax check catches.
func zshOptionSpec(declared option.Option, excluded []option.Option) string {
	forms := names(declared)
	if len(forms) == 0 {
		return ""
	}

	// What giving this option rules out: its own other spellings, and anything an exclusive group
	// puts against it. Without this a shell offers --verbose after --quiet, which the parser would
	// then refuse.
	prefix := ""

	if repeatable(declared) {
		// An option that accumulates may be offered again, which the leading star is what says.
		prefix = "*"
	} else {
		ruledOut := make([]string, 0, len(forms)+len(excluded))
		ruledOut = append(ruledOut, forms...)
		for _, other := range excluded {
			ruledOut = append(ruledOut, names(other)...)
		}

		if len(ruledOut) != 0 {
			prefix = "(" + strings.Join(ruledOut, " ") + ")"
		}
	}

	// A value that may be attached to the option rather than following it. The equals form is what
	// lets --option=value complete.
	suffix := ""
	if takesArgument(declared) {
		if optionalArgument(declared) {
			suffix = "=-"
		} else {
			suffix = "+"
		}
	}

	tail := suffix + "[" + zshDescription(summary(declared.GetUsage())) + "]" + zshValueSpec(declared)

	// One form is one argument, so the whole spec is quoted together. Two forms need the braces
	// outside the quotes, so that the shell expands them before _arguments reads them.
	if len(forms) == 2 {
		return zshQuote(prefix) + "{" + forms[0] + "," + forms[1] + "}" + zshQuote(tail)
	}

	return zshQuote(prefix + forms[0] + tail)
}

// zshPositionalSpec renders a positional argument.
func zshPositionalSpec(declared option.Option) string {
	name := zshDescription(metavar(declared))

	prefix := ":"
	// A positional that may be given repeatedly is offered for as long as it accepts more.
	if declared.GetNargs().IsVariadic() {
		prefix = "*:"
	}

	if accepted := choices(declared); len(accepted) != 0 {
		escaped := make([]string, 0, len(accepted))
		for _, choice := range accepted {
			escaped = append(escaped, zshDescription(choice))
		}

		return prefix + name + ":(" + strings.Join(escaped, " ") + ")"
	}

	switch strings.ToUpper(metavar(declared)) {
	case "PATH", "FILE":
		return prefix + name + ":_files"
	case "DIR", "DIRECTORY":
		return prefix + name + ":_directories"
	default:
		return prefix + name + ":"
	}
}

// zshSpecs renders every spec a parser's own options and positionals produce.
func zshSpecs(parser *argument_parser.Parser) []string {
	ruledOut := exclusions(parser)

	specs := make([]string, 0, len(parser.Options)+len(parser.Positionals)+1)

	for _, declared := range parser.Options {
		if declared == nil || hidden(declared) {
			continue
		}

		if spec := zshOptionSpec(declared, ruledOut[declared]); spec != "" {
			specs = append(specs, spec)
		}
	}

	// The parser adds --help itself unless told not to, so a completion that did not offer it
	// would be missing an option the program accepts.
	if !parser.DisableHelp {
		specs = append(specs, `'(-h --help)'{-h,--help}'[Show this help message and exit]'`)
	}

	for _, declared := range parser.Positionals {
		if declared == nil || hidden(declared) {
			continue
		}

		specs = append(specs, zshQuote(zshPositionalSpec(declared)))
	}

	return specs
}

// writeZsh writes a zsh completion for the parser.
func writeZsh(writer io.Writer, parser *argument_parser.Parser) error {
	name := parser.ProgramName
	function := "_" + zshIdentifier(name)

	var builder strings.Builder

	builder.WriteString("#compdef " + name + "\n")
	builder.WriteString("# Generated from the parser's own declaration. Do not edit.\n\n")

	commands := subcommands(parser)

	// A subcommand's options are written as a function of its own, which the dispatch below calls.
	for _, entry := range commands {
		if entry.parser == nil {
			continue
		}

		fmt.Fprintf(&builder, "%s_%s() {\n", function, zshIdentifier(entry.name))
		builder.WriteString("  _arguments -s \\\n")
		writeZshSpecLines(&builder, zshSpecs(entry.parser))
		builder.WriteString("}\n\n")
	}

	builder.WriteString(function + "() {\n")

	if len(commands) == 0 {
		builder.WriteString("  _arguments -s \\\n")
		writeZshSpecLines(&builder, zshSpecs(parser))
		builder.WriteString("}\n\n")
		builder.WriteString(function + ` "$@"` + "\n")

		_, err := io.WriteString(writer, builder.String())
		if err != nil {
			return fmt.Errorf("io write string: %w", err)
		}

		return nil
	}

	builder.WriteString("  local curcontext=\"$curcontext\" state line\n")
	builder.WriteString("  typeset -A opt_args\n\n")
	builder.WriteString("  _arguments -s -C \\\n")

	specs := zshSpecs(parser)
	specs = append(specs, zshQuote(": :->command"), zshQuote("*:: :->argument"))
	writeZshSpecLines(&builder, specs)

	builder.WriteString("\n  case $state in\n")
	builder.WriteString("    command)\n")
	builder.WriteString("      local -a commands\n")
	builder.WriteString("      commands=(\n")

	for _, entry := range commands {
		builder.WriteString(
			"        " + zshQuote(entry.name+":"+zshDescription(summary(entry.description))) + "\n",
		)
	}

	builder.WriteString("      )\n")
	builder.WriteString("      _describe -t commands 'command' commands\n")
	builder.WriteString("      ;;\n")
	builder.WriteString("    argument)\n")
	builder.WriteString("      case $words[1] in\n")

	for _, entry := range commands {
		builder.WriteString("        " + entry.name + ")\n")
		if entry.parser != nil {
			fmt.Fprintf(&builder, "          %s_%s\n", function, zshIdentifier(entry.name))
		}
		builder.WriteString("          ;;\n")
	}

	builder.WriteString("      esac\n")
	builder.WriteString("      ;;\n")
	builder.WriteString("  esac\n")
	builder.WriteString("}\n\n")
	builder.WriteString(function + ` "$@"` + "\n")

	if _, err := io.WriteString(writer, builder.String()); err != nil {
		return fmt.Errorf("io write string: %w", err)
	}

	return nil
}

// writeZshSpecLines writes the specs as the continued lines of an _arguments call.
//
// The specs arrive quoted, because how each is quoted depends on its shape: a spec with two forms
// has to leave its brace expansion outside the quotes.
func writeZshSpecLines(builder *strings.Builder, specs []string) {
	for index, spec := range specs {
		builder.WriteString("    " + spec)
		if index != len(specs)-1 {
			builder.WriteString(" \\")
		}
		builder.WriteString("\n")
	}
}
