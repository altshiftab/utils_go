package main

import (
	"flag"
	"fmt"
	"os"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	motmedelSbom "github.com/altshiftab/utils_go/pkg/sbom"
	motmedelSbomTypes "github.com/altshiftab/utils_go/pkg/sbom/types"
)

func run() error {
	var goSumPath string
	var nodeLockPath string
	var dockerfilePath string
	var outputPath string

	flag.StringVar(&goSumPath, "go", "", "path to go.sum file")
	flag.StringVar(&nodeLockPath, "node", "", "path to package-lock.json file")
	flag.StringVar(&dockerfilePath, "docker", "", "path to Dockerfile")
	flag.StringVar(&outputPath, "output", "", "output file path (default: stdout)")
	flag.Parse()

	var allComponents []motmedelSbomTypes.Component

	if goSumPath != "" {
		data, err := os.ReadFile(goSumPath)
		if err != nil {
			return &motmedelErrors.Error{
				Message: "An error occurred when reading the go.sum file.",
				Cause:   err,
				Input:   goSumPath,
			}
		}

		components, err := motmedelSbom.ParseGoSum(data)
		if err != nil {
			return &motmedelErrors.Error{
				Message: "An error occurred when parsing the go.sum file.",
				Cause:   err,
				Input:   goSumPath,
			}
		}

		allComponents = append(allComponents, components...)
	}

	if nodeLockPath != "" {
		data, err := os.ReadFile(nodeLockPath)
		if err != nil {
			return &motmedelErrors.Error{
				Message: "An error occurred when reading the package-lock.json file.",
				Cause:   err,
				Input:   nodeLockPath,
			}
		}

		components, err := motmedelSbom.ParseNodePackageLock(data)
		if err != nil {
			return &motmedelErrors.Error{
				Message: "An error occurred when parsing the package-lock.json file.",
				Cause:   err,
				Input:   nodeLockPath,
			}
		}

		allComponents = append(allComponents, components...)
	}

	if dockerfilePath != "" {
		data, err := os.ReadFile(dockerfilePath)
		if err != nil {
			return &motmedelErrors.Error{
				Message: "An error occurred when reading the Dockerfile.",
				Cause:   err,
				Input:   dockerfilePath,
			}
		}

		components, err := motmedelSbom.ParseDockerfile(data)
		if err != nil {
			return &motmedelErrors.Error{
				Message: "An error occurred when parsing the Dockerfile.",
				Cause:   err,
				Input:   dockerfilePath,
			}
		}

		allComponents = append(allComponents, components...)
	}

	output, err := motmedelSbom.GenerateBomJson(allComponents)
	if err != nil {
		return &motmedelErrors.Error{
			Message: "An error occurred when generating the SBOM JSON.",
			Cause:   err,
		}
	}

	if outputPath != "" {
		if err := os.WriteFile(outputPath, output, 0600); err != nil { //nolint:gosec // G703: writing to the user-chosen output path is the CLI's purpose.
			return &motmedelErrors.Error{
				Message: "An error occurred when writing the output file.",
				Cause:   err,
				Input:   outputPath,
			}
		}
	} else {
		fmt.Print(string(output))
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
