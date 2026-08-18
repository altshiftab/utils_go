// Command sbom writes a CycloneDX SBOM of a container image and what it was built with: the packages inside the built
// image (OS packages, the modules linked into Go executables, shipped node packages), the contents of the images the
// Dockerfile's build stages start from (excluded from the delivered artifact, but part of how it was produced), Go
// executables given directly, and the packages of a package-lock.json.
package main

import (
	"context"
	"debug/buildinfo"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	argumentParser "github.com/altshiftab/utils_go/pkg/cli/argument_parser"
	argumentParserErrors "github.com/altshiftab/utils_go/pkg/cli/argument_parser/errors"
	"github.com/altshiftab/utils_go/pkg/cli/argument_parser/option"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftSbom "github.com/altshiftab/utils_go/pkg/sbom"
	altshiftSbomImage "github.com/altshiftab/utils_go/pkg/sbom/image"
	altshiftSbomTypes "github.com/altshiftab/utils_go/pkg/sbom/types"
)

const (
	exitClean = 0
	exitError = 1
	exitUsage = 2
)

// errNothingToDescribe is the usage error for a command line that names no input.
var errNothingToDescribe = errors.New("nothing to describe: give at least one of --image, --dockerfile, --go-binary, --node")

const description = "Write a CycloneDX SBOM of a container image and what it was built with. " +
	"The built image (--image) is read from the local podman store; its OS packages, the modules linked into its Go " +
	"executables and its node packages are listed as required components. The images the Dockerfile's build stages " +
	"start from (--dockerfile) are read the same way and listed as excluded container components holding their " +
	"packages; what RUN instructions installed on top of them is not seen. The final stage's base image is only " +
	"named, its content being the built image's."

type arguments struct {
	image      string
	dockerfile string
	goBinaries []string
	nodeLock   string
	output     string
	podman     string
}

func newParser(args *arguments, output io.Writer) *argumentParser.Parser {
	return &argumentParser.Parser{
		ProgramName: "sbom",
		Description: description,
		Output:      output,
		Options: []option.Option{
			option.WithMetavar(option.NewStringOption(0, "image", "the built image to describe, as podman knows it (e.g. localhost/app:latest); it becomes the SBOM's subject", false, &args.image), "REFERENCE"),
			option.WithMetavar(option.NewStringOption(0, "dockerfile", "the Dockerfile the image was built from; its build-stage images are analyzed as excluded components", false, &args.dockerfile), "PATH"),
			option.WithMetavar(option.NewStringsOption(0, "go-binary", "a Go executable whose linked modules to list (may be repeated)", false, &args.goBinaries), "PATH"),
			option.WithMetavar(option.NewStringOption(0, "node", "a package-lock.json whose packages to list; development dependencies are excluded components", false, &args.nodeLock), "PATH"),
			option.WithMetavar(option.NewStringOption(0, "output", "the file to write the SBOM to (default: standard output)", false, &args.output), "PATH"),
			option.WithMetavar(option.WithDefault(option.NewStringOption(0, "podman", "the podman executable to read images with", false, &args.podman), "podman"), "PATH"),
		},
		// The accepted spellings stay exactly the names as written, so that a Makefile that spells one keeps
		// working when an option is added later.
		DisableAbbrev: true,
	}
}

// run does what the command line asks and returns the exit code and, for a failure, the error to report.
func run(ctx context.Context, argv []string, stdout, stderr io.Writer) (int, error) {
	args := &arguments{}
	parser := newParser(args, stdout)
	if err := parser.Validate(); err != nil {
		return exitError, altshiftErrors.New(fmt.Errorf("parser validate: %w", err))
	}
	if err := parser.ParseArgs(argv); err != nil {
		if errors.Is(err, argumentParserErrors.ErrHelp) {
			return exitClean, nil
		}
		fmt.Fprint(stderr, parser.FormatError(err))
		return exitUsage, nil
	}

	if args.image == "" && args.dockerfile == "" && len(args.goBinaries) == 0 && args.nodeLock == "" {
		fmt.Fprint(stderr, parser.FormatError(errNothingToDescribe))
		return exitUsage, nil
	}

	store := &altshiftSbomImage.Store{Podman: args.podman}

	var subject *altshiftSbomTypes.Component
	var components []*altshiftSbomTypes.Component
	warn := func(reference string, warnings []string) {
		for _, warning := range warnings {
			fmt.Fprintf(stderr, "sbom: warning: %s: %s\n", reference, warning)
		}
	}

	if args.image != "" {
		analysis, err := store.Analyze(ctx, args.image)
		if err != nil {
			return exitError, altshiftErrors.New(fmt.Errorf("analyze image %q: %w", args.image, err), args.image)
		}
		warn(args.image, analysis.Warnings)
		subject = altshiftSbom.ContainerComponent(args.image, analysis, "")
		imageComponents, err := altshiftSbom.ImageComponents(analysis, altshiftSbomTypes.ScopeRequired)
		if err != nil {
			return exitError, altshiftErrors.New(fmt.Errorf("image components (%s): %w", args.image, err), args.image)
		}
		components = append(components, imageComponents...)
	}

	for _, goBinary := range args.goBinaries {
		info, err := buildinfo.ReadFile(goBinary)
		if err != nil {
			return exitError, altshiftErrors.NewWithTrace(fmt.Errorf("read build info of %q: %w", goBinary, err), goBinary)
		}
		components = append(components, altshiftSbom.BuildInfoComponents(info, goBinary, altshiftSbomTypes.ScopeRequired)...)
	}

	if args.nodeLock != "" {
		data, err := os.ReadFile(args.nodeLock)
		if err != nil {
			return exitError, altshiftErrors.NewWithTrace(fmt.Errorf("read %q: %w", args.nodeLock, err), args.nodeLock)
		}
		nodeComponents, err := altshiftSbom.ParseNodePackageLock(data)
		if err != nil {
			return exitError, altshiftErrors.New(fmt.Errorf("parse package-lock %q: %w", args.nodeLock, err), args.nodeLock)
		}
		components = append(components, nodeComponents...)
	}

	if args.dockerfile != "" {
		data, err := os.ReadFile(args.dockerfile)
		if err != nil {
			return exitError, altshiftErrors.NewWithTrace(fmt.Errorf("read %q: %w", args.dockerfile, err), args.dockerfile)
		}
		stages, err := altshiftSbom.ParseDockerfileStages(data)
		if err != nil {
			return exitError, altshiftErrors.New(fmt.Errorf("parse dockerfile %q: %w", args.dockerfile, err), args.dockerfile)
		}
		buildImages, finalBase := altshiftSbom.DockerfileImages(stages)

		for _, buildImage := range buildImages {
			// A FROM naming a build argument ("${BASE_IMAGE}") cannot be resolved without the build's arguments;
			// it is named as written and its contents left out, which the warning says.
			if strings.Contains(buildImage, "$") {
				warn(buildImage, []string{"image reference holds a build argument; its contents are not listed"})
				components = append(components, altshiftSbom.ContainerComponent(buildImage, nil, altshiftSbomTypes.ScopeExcluded))
				continue
			}
			analysis, err := store.Analyze(ctx, buildImage)
			if err != nil {
				return exitError, altshiftErrors.New(fmt.Errorf("analyze build image %q: %w", buildImage, err), buildImage)
			}
			warn(buildImage, analysis.Warnings)
			container := altshiftSbom.ContainerComponent(buildImage, analysis, altshiftSbomTypes.ScopeExcluded)
			imageComponents, err := altshiftSbom.ImageComponents(analysis, altshiftSbomTypes.ScopeExcluded)
			if err != nil {
				return exitError, altshiftErrors.New(fmt.Errorf("image components (%s): %w", buildImage, err), buildImage)
			}
			components = append(components, altshiftSbom.Nest(container, imageComponents))
		}
		if finalBase != "" {
			components = append(components, altshiftSbom.ContainerComponent(finalBase, nil, altshiftSbomTypes.ScopeRequired))
		}
	}

	output, err := altshiftSbom.GenerateBomJson(subject, components)
	if err != nil {
		return exitError, fmt.Errorf("generate bom json: %w", err)
	}

	if args.output != "" {
		if err := os.WriteFile(args.output, output, 0o600); err != nil { //nolint:gosec // G703: writing to the user-chosen output path is the CLI's purpose.
			return exitError, altshiftErrors.NewWithTrace(fmt.Errorf("write %q: %w", args.output, err), args.output)
		}
		return exitClean, nil
	}

	if _, err := stdout.Write(output); err != nil {
		return exitError, altshiftErrors.NewWithTrace(fmt.Errorf("write stdout: %w", err))
	}
	return exitClean, nil
}

func main() {
	// Interrupting the command interrupts the podman it is reading from.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	code, err := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sbom: error: %v\n", err)
	}
	os.Exit(code)
}
