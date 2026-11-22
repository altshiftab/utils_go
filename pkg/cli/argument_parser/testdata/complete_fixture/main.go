// Command complete_fixture is a program built on the parser, for a test to ask for completions as
// a shell does. Its body leaves a file where FIXTURE_SENTINEL says, so that the test sees whether
// it ran.
package main

import (
	"context"
	"os"
	"strings"

	"github.com/altshiftab/utils_go/pkg/cli/argument_parser"
	"github.com/altshiftab/utils_go/pkg/cli/argument_parser/option"
)

func main() {
	var title string

	parser := &argument_parser.Parser{
		ProgramName: "complete_fixture",
		Positionals: []option.Option{
			option.WithCompleter(
				option.WithMetavar(option.NewStringOption(0, "", "A title.", true, &title), "TITLE"),
				func(_ context.Context, prefix string) ([]*option.Candidate, error) {
					var found []*option.Candidate
					for _, candidate := range []string{"Secrets", "Black Bag"} {
						if strings.HasPrefix(candidate, prefix) {
							found = append(found, &option.Candidate{Value: candidate})
						}
					}
					return found, nil
				},
			),
		},
	}
	parser.ParseOrExit()

	if err := os.WriteFile(os.Getenv("FIXTURE_SENTINEL"), []byte(title), 0o600); err != nil {
		os.Exit(1)
	}
}
