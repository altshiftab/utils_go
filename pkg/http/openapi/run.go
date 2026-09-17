package openapi

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/altshiftab/utils_go/pkg/cli/argument_parser"
	"github.com/altshiftab/utils_go/pkg/cli/argument_parser/option"
	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	openapiErrors "github.com/altshiftab/utils_go/pkg/http/openapi/errors"
	"github.com/altshiftab/utils_go/pkg/http/openapi/openapi_config"
)

// Exit codes. Two is what a command line that could not be read exits with, as the conventions have
// it; one is a document that could not be written, or one that has drifted.
const (
	exitSuccess  = 0
	exitFailure  = 1
	exitBadUsage = 2
)

// stdoutPath is the output naming standard output rather than a file.
const stdoutPath = "-"

// Run writes the document describing the endpoints, as a program.
//
// The options carry what no endpoint can supply -- the title, the version, the security schemes --
// and the arguments carry what belongs on a command line. A caller is a main of a dozen lines that
// hands over its own endpoints, so that the flags, the writing and the drift check are written once
// rather than per service.
func Run(arguments []string, endpoints []*endpointPkg.Endpoint, options ...openapi_config.Option) int {
	var outPath string
	var specVersion string
	var check bool

	parser := &argument_parser.Parser{
		Description: "Write the OpenAPI document describing the service's endpoints.",
		// The accepted command line stays the names as written: a document generator is called from
		// a Makefile, and adding an option later must not change what an existing call means.
		DisableAbbrev: true,
		Options: []option.Option{
			option.NewStringOption(
				'o',
				"out",
				"Write the document to this file, or to standard output for \"-\".",
				false,
				&outPath,
			),
			option.WithChoices(
				option.NewStringOption(
					0,
					"openapi-version",
					"The specification version to write against.",
					false,
					&specVersion,
				),
				openapi_config.Version32,
				openapi_config.Version31,
			),
			option.NewBoolOption(
				0,
				"check",
				"Write nothing; exit non-zero if the file at --out differs from what would be written.",
				false,
				&check,
			),
		},
	}

	if err := parser.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "the argument parser is not valid: %v\n", err)

		return exitBadUsage
	}

	if err := parser.ParseArgs(arguments); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)

		return exitBadUsage
	}

	if specVersion != "" {
		// Appended to a copy: the caller's slice may have spare capacity, and writing into it would
		// change what the caller holds.
		options = append(slices.Clone(options), openapi_config.WithVersion(specVersion))
	}

	document, err := Generate(endpoints, options...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "the document could not be generated: %v\n", err)

		return exitFailure
	}

	data, err := Marshal(document)
	if err != nil {
		fmt.Fprintf(os.Stderr, "the document could not be written: %v\n", err)

		return exitFailure
	}

	if check {
		if err := checkFile(outPath, data); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)

			return exitFailure
		}

		return exitSuccess
	}

	if outPath == "" || outPath == stdoutPath {
		if _, err := os.Stdout.Write(data); err != nil {
			fmt.Fprintf(os.Stderr, "the document could not be written to standard output: %v\n", err)

			return exitFailure
		}

		return exitSuccess
	}

	if err := os.WriteFile(outPath, data, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "the document could not be written to %s: %v\n", outPath, err)

		return exitFailure
	}

	return exitSuccess
}

// checkFile reports whether the committed document is still the one the endpoints produce.
//
// It is the drift check a build runs: a document is committed so that a change to what the service
// accepts shows up as a diff in review, and a committed copy nobody regenerates is worse than none.
func checkFile(path string, data []byte) error {
	if path == "" || path == stdoutPath {
		return fmt.Errorf("%w: --check needs a file to check, given by --out", openapiErrors.ErrDocumentDrift)
	}

	existing, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s does not exist", openapiErrors.ErrDocumentDrift, path)
		}

		return fmt.Errorf("the document at %s could not be read: %w", path, err)
	}

	if !bytes.Equal(existing, data) {
		return fmt.Errorf("%w: %s is not what the endpoints produce", openapiErrors.ErrDocumentDrift, path)
	}

	return nil
}
