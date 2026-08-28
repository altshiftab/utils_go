package completion

import (
	"fmt"
	"io"
	"strings"

	"github.com/altshiftab/utils_go/pkg/cli/argument_parser"
)

// bashQuote wraps a string in single quotes, closing and reopening them around any it contains.
func bashQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// bashIdentifier is the name of a generated function.
func bashIdentifier(value string) string {
	return zshIdentifier(value)
}

// bashOptionNames returns every form of every option a parser declares, as a space-separated list.
func bashOptionNames(parser *argument_parser.Parser) string {
	found := make([]string, 0, len(parser.Options)*2)

	for _, declared := range parser.Options {
		if declared == nil || hidden(declared) {
			continue
		}

		found = append(found, names(declared)...)
	}

	if !parser.DisableHelp {
		found = append(found, "-h", "--help")
	}

	return strings.Join(found, " ")
}

// bashChoiceCases writes a case arm per option that accepts only certain values, so that the shell
// offers those values after the option rather than the next option's name.
func bashChoiceCases(builder *strings.Builder, parser *argument_parser.Parser, indent string) {
	wrote := false

	for _, declared := range parser.Options {
		if declared == nil || hidden(declared) || !takesArgument(declared) {
			continue
		}

		accepted := choices(declared)
		if len(accepted) == 0 {
			continue
		}

		forms := names(declared)
		if len(forms) == 0 {
			continue
		}

		if !wrote {
			builder.WriteString(indent + "case \"$previous\" in\n")
			wrote = true
		}

		builder.WriteString(indent + "  " + strings.Join(forms, "|") + ")\n")
		builder.WriteString(
			indent + "    COMPREPLY=($(compgen -W " + bashQuote(strings.Join(accepted, " ")) + ` -- "$current"))` + "\n",
		)
		builder.WriteString(indent + "    return 0\n")
		builder.WriteString(indent + "    ;;\n")
	}

	if wrote {
		builder.WriteString(indent + "esac\n\n")
	}
}

// writeBash writes a bash completion for the parser.
//
// It says less than the zsh one, and the difference is the shell's rather than this package's: bash
// completes from a flat list of words, with no room for a description beside each or for one option
// ruling out another. What it does carry is the option names, and the values of an option that
// accepts only some.
func writeBash(writer io.Writer, parser *argument_parser.Parser) error {
	name := parser.ProgramName
	function := "_" + bashIdentifier(name)

	var builder strings.Builder

	builder.WriteString("# Generated from the parser's own declaration. Do not edit.\n")
	builder.WriteString("# bash completes from a flat list of words, so the descriptions and the\n")
	builder.WriteString("# exclusions the declaration carries are not expressible here; the zsh\n")
	builder.WriteString("# completion has them.\n\n")

	builder.WriteString(function + "() {\n")
	builder.WriteString("  local current previous\n")
	builder.WriteString("  current=\"${COMP_WORDS[COMP_CWORD]}\"\n")
	builder.WriteString("  previous=\"${COMP_WORDS[COMP_CWORD-1]}\"\n\n")

	commands := subcommands(parser)

	if len(commands) == 0 {
		bashChoiceCases(&builder, parser, "  ")
		builder.WriteString(
			"  COMPREPLY=($(compgen -W " + bashQuote(bashOptionNames(parser)) + ` -- "$current"))` + "\n",
		)
		builder.WriteString("}\n\n")
		fmt.Fprintf(&builder, "complete -F %s %s\n", function, name)

		if _, err := io.WriteString(writer, builder.String()); err != nil {
			return fmt.Errorf("io write string: %w", err)
		}

		return nil
	}

	commandNames := make([]string, 0, len(commands))
	for _, entry := range commands {
		commandNames = append(commandNames, entry.name)
	}

	// The program's own options that accept only certain values are handled before anything else,
	// because they may be given before the subcommand: after --completion, the shell should offer
	// the shells rather than the list of commands.
	bashChoiceCases(&builder, parser, "  ")

	// Which subcommand was given, if any. Everything after it completes against that subcommand's
	// options rather than the program's.
	builder.WriteString("  local command=\"\"\n")
	builder.WriteString("  local index\n")
	builder.WriteString("  for ((index = 1; index < COMP_CWORD; index++)); do\n")
	builder.WriteString("    case \"${COMP_WORDS[index]}\" in\n")
	builder.WriteString("      " + strings.Join(commandNames, "|") + ")\n")
	builder.WriteString("        command=\"${COMP_WORDS[index]}\"\n")
	builder.WriteString("        break\n")
	builder.WriteString("        ;;\n")
	builder.WriteString("    esac\n")
	builder.WriteString("  done\n\n")

	builder.WriteString("  if [[ -z $command ]]; then\n")
	builder.WriteString(
		"    COMPREPLY=($(compgen -W " +
			bashQuote(strings.Join(commandNames, " ")+" "+bashOptionNames(parser)) +
			` -- "$current"))` + "\n",
	)
	builder.WriteString("    return 0\n")
	builder.WriteString("  fi\n\n")

	builder.WriteString("  case \"$command\" in\n")

	for _, entry := range commands {
		builder.WriteString("    " + entry.name + ")\n")

		if entry.parser == nil {
			builder.WriteString("      ;;\n")
			continue
		}

		bashChoiceCases(&builder, entry.parser, "      ")
		builder.WriteString(
			"      COMPREPLY=($(compgen -W " +
				bashQuote(bashOptionNames(entry.parser)) + ` -- "$current"))` + "\n",
		)
		builder.WriteString("      ;;\n")
	}

	builder.WriteString("  esac\n")
	builder.WriteString("}\n\n")
	fmt.Fprintf(&builder, "complete -F %s %s\n", function, name)

	if _, err := io.WriteString(writer, builder.String()); err != nil {
		return fmt.Errorf("io write string: %w", err)
	}

	return nil
}
