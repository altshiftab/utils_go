package main

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftSbomTypes "github.com/altshiftab/utils_go/pkg/sbom/types"
)

const (
	library   = altshiftSbomTypes.ComponentTypeLibrary
	container = altshiftSbomTypes.ComponentTypeContainer
)

// runMain invokes run() with a controlled argv, capturing anything written to
// stdout. The global flag set, os.Args and os.Stdout are saved and restored so
// the call can be repeated. These tests must not run in parallel because they
// mutate this process-wide state.
func runMain(t *testing.T, args ...string) (string, error) {
	t.Helper()

	origArgs := os.Args
	origCommandLine := flag.CommandLine
	origStdout := os.Stdout
	defer func() {
		os.Args = origArgs
		flag.CommandLine = origCommandLine
		os.Stdout = origStdout
	}()

	os.Args = append([]string{"sbom"}, args...)
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	os.Stdout = writeEnd

	captured := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, readEnd)
		captured <- buf.String()
	}()

	runErr := run()

	_ = writeEnd.Close()
	stdout := <-captured
	_ = readEnd.Close()

	return stdout, runErr
}

func fileNameForFlag(flagName string) string {
	switch flagName {
	case "go":
		return "go.sum"
	case "node":
		return "package-lock.json"
	case "docker":
		return "Dockerfile"
	default:
		return flagName
	}
}

func componentsByName(components []altshiftSbomTypes.Component) map[string]altshiftSbomTypes.Component {
	byName := make(map[string]altshiftSbomTypes.Component, len(components))
	for _, component := range components {
		byName[component.Name] = component
	}
	return byName
}

const goSumFixture = "github.com/example/foo v1.2.3 h1:aaa=\n" +
	"github.com/example/foo v1.2.3/go.mod h1:aaa=\n" +
	"github.com/example/bar v0.1.0 h1:bbb=\n" +
	"github.com/example/bar v0.1.0/go.mod h1:bbb=\n"

const packageLockFixture = `{
  "name": "app",
  "version": "1.0.0",
  "packages": {
    "": {"name": "app", "version": "1.0.0"},
    "node_modules/left-pad": {"version": "1.3.0", "license": "MIT"},
    "node_modules/@scope/util": {"version": "2.0.0"}
  }
}`

const dockerfileFixture = "FROM golang:1.22 AS build\n" +
	"FROM alpine:3.19\n" +
	"FROM scratch\n"

type wantComponent struct {
	typ     altshiftSbomTypes.ComponentType
	version string
	purl    string
}

// These tests mutate process globals (os.Args, flag.CommandLine, os.Stdout) via
// runMain and therefore cannot run in parallel.
func TestRunGeneratesBom(t *testing.T) { //nolint:paralleltest // shares process-global state through run()
	testCases := []struct {
		name           string
		files          map[string]string
		toFile         bool
		wantComponents map[string]wantComponent
	}{
		{
			name:   "go.sum written to output file",
			files:  map[string]string{"go": goSumFixture},
			toFile: true,
			wantComponents: map[string]wantComponent{
				"github.com/example/foo": {typ: library, version: "v1.2.3", purl: "pkg:golang/github.com/example/foo@v1.2.3"},
				"github.com/example/bar": {typ: library, version: "v0.1.0", purl: "pkg:golang/github.com/example/bar@v0.1.0"},
			},
		},
		{
			name:  "dockerfile written to stdout",
			files: map[string]string{"docker": dockerfileFixture},
			wantComponents: map[string]wantComponent{
				"golang": {typ: container, version: "1.22", purl: "pkg:docker/golang@1.22"},
				"alpine": {typ: container, version: "3.19", purl: "pkg:docker/alpine@3.19"},
			},
		},
		{
			name:   "package-lock written to output file",
			files:  map[string]string{"node": packageLockFixture},
			toFile: true,
			wantComponents: map[string]wantComponent{
				"left-pad":    {typ: library, version: "1.3.0", purl: "pkg:npm/left-pad@1.3.0"},
				"@scope/util": {typ: library, version: "2.0.0", purl: "pkg:npm/%40scope/util@2.0.0"},
			},
		},
		{
			name:   "all sources combined to output file",
			files:  map[string]string{"go": goSumFixture, "node": packageLockFixture, "docker": dockerfileFixture},
			toFile: true,
			wantComponents: map[string]wantComponent{
				"github.com/example/foo": {typ: library, version: "v1.2.3", purl: "pkg:golang/github.com/example/foo@v1.2.3"},
				"github.com/example/bar": {typ: library, version: "v0.1.0", purl: "pkg:golang/github.com/example/bar@v0.1.0"},
				"left-pad":               {typ: library, version: "1.3.0", purl: "pkg:npm/left-pad@1.3.0"},
				"@scope/util":            {typ: library, version: "2.0.0", purl: "pkg:npm/%40scope/util@2.0.0"},
				"golang":                 {typ: container, version: "1.22", purl: "pkg:docker/golang@1.22"},
				"alpine":                 {typ: container, version: "3.19", purl: "pkg:docker/alpine@3.19"},
			},
		},
		{
			name:           "no inputs produces empty component list",
			files:          nil,
			toFile:         false,
			wantComponents: map[string]wantComponent{},
		},
	}

	for _, testCase := range testCases { //nolint:paralleltest // shares process-global state through run()
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()

			var args []string
			for flagName, content := range testCase.files {
				path := filepath.Join(dir, fileNameForFlag(flagName))
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatalf("failed to write %s fixture: %v", flagName, err)
				}
				args = append(args, "-"+flagName+"="+path)
			}

			var outputPath string
			if testCase.toFile {
				outputPath = filepath.Join(dir, "sbom.json")
				args = append(args, "-output="+outputPath)
			}

			stdout, err := runMain(t, args...)
			if err != nil {
				t.Fatalf("run() returned unexpected error: %v", err)
			}

			var data []byte
			if testCase.toFile {
				if stdout != "" {
					t.Errorf("expected no stdout when writing to a file, got %q", stdout)
				}

				fileData, readErr := os.ReadFile(outputPath)
				if readErr != nil {
					t.Fatalf("failed to read output file: %v", readErr)
				}
				data = fileData
			} else {
				data = []byte(stdout)
			}

			var bom altshiftSbomTypes.Bom
			if err := json.Unmarshal(data, &bom); err != nil {
				t.Fatalf("failed to unmarshal BOM JSON: %v\noutput:\n%s", err, string(data))
			}

			if bom.BomFormat != altshiftSbomTypes.BomFormatCycloneDX {
				t.Errorf("bomFormat = %q, want %q", bom.BomFormat, altshiftSbomTypes.BomFormatCycloneDX)
			}
			if bom.SpecVersion != "1.6" {
				t.Errorf("specVersion = %q, want %q", bom.SpecVersion, "1.6")
			}
			if bom.Version != 1 {
				t.Errorf("version = %d, want 1", bom.Version)
			}
			if bom.Metadata == nil {
				t.Fatalf("metadata is nil")
			}
			if len(bom.Metadata.Tools) != 1 {
				t.Fatalf("metadata tools count = %d, want 1", len(bom.Metadata.Tools))
			}
			if toolName := bom.Metadata.Tools[0].Name; toolName != "altshift-sbom-generator" {
				t.Errorf("tool name = %q, want %q", toolName, "altshift-sbom-generator")
			}

			if len(bom.Components) != len(testCase.wantComponents) {
				t.Errorf(
					"component count = %d, want %d; got components: %+v",
					len(bom.Components), len(testCase.wantComponents), bom.Components,
				)
			}

			byName := componentsByName(bom.Components)
			for name, want := range testCase.wantComponents {
				got, ok := byName[name]
				if !ok {
					t.Errorf("missing expected component %q", name)
					continue
				}
				if got.Type != want.typ {
					t.Errorf("component %q type = %q, want %q", name, got.Type, want.typ)
				}
				if got.Version != want.version {
					t.Errorf("component %q version = %q, want %q", name, got.Version, want.version)
				}
				if got.Purl != want.purl {
					t.Errorf("component %q purl = %q, want %q", name, got.Purl, want.purl)
				}
				if got.BomRef != want.purl {
					t.Errorf("component %q bom-ref = %q, want %q", name, got.BomRef, want.purl)
				}
			}
		})
	}
}

func TestRunErrors(t *testing.T) { //nolint:paralleltest // shares process-global state through run()
	testCases := []struct {
		name        string
		setup       func(t *testing.T, dir string) []string
		wantMessage string
		wantInput   func(dir string) string
	}{
		{
			name: "missing go.sum file",
			setup: func(_ *testing.T, dir string) []string {
				return []string{"-go=" + filepath.Join(dir, "missing.sum")}
			},
			wantMessage: "An error occurred when reading the go.sum file.",
			wantInput:   func(dir string) string { return filepath.Join(dir, "missing.sum") },
		},
		{
			name: "missing package-lock file",
			setup: func(_ *testing.T, dir string) []string {
				return []string{"-node=" + filepath.Join(dir, "missing.json")}
			},
			wantMessage: "An error occurred when reading the package-lock.json file.",
			wantInput:   func(dir string) string { return filepath.Join(dir, "missing.json") },
		},
		{
			name: "missing dockerfile",
			setup: func(_ *testing.T, dir string) []string {
				return []string{"-docker=" + filepath.Join(dir, "missing.dockerfile")}
			},
			wantMessage: "An error occurred when reading the Dockerfile.",
			wantInput:   func(dir string) string { return filepath.Join(dir, "missing.dockerfile") },
		},
		{
			name: "invalid package-lock JSON",
			setup: func(t *testing.T, dir string) []string {
				path := filepath.Join(dir, "package-lock.json")
				if err := os.WriteFile(path, []byte("{ this is not valid json"), 0o600); err != nil {
					t.Fatalf("failed to write invalid package-lock fixture: %v", err)
				}
				return []string{"-node=" + path}
			},
			wantMessage: "An error occurred when parsing the package-lock.json file.",
			wantInput:   func(dir string) string { return filepath.Join(dir, "package-lock.json") },
		},
		{
			name: "unwritable output path",
			setup: func(_ *testing.T, dir string) []string {
				return []string{"-output=" + filepath.Join(dir, "no-such-dir", "sbom.json")}
			},
			wantMessage: "An error occurred when writing the output file.",
			wantInput:   func(dir string) string { return filepath.Join(dir, "no-such-dir", "sbom.json") },
		},
	}

	for _, testCase := range testCases { //nolint:paralleltest // shares process-global state through run()
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			args := testCase.setup(t, dir)

			_, err := runMain(t, args...)
			if err == nil {
				t.Fatalf("expected run() to return an error, got nil")
			}

			altshiftErr, ok := errors.AsType[*altshiftErrors.Error](err)
			if !ok {
				t.Fatalf("expected a *altshiftErrors.Error, got %T: %v", err, err)
			}

			if altshiftErr.Message != testCase.wantMessage {
				t.Errorf("error message = %q, want %q", altshiftErr.Message, testCase.wantMessage)
			}
			if altshiftErr.Cause == nil {
				t.Errorf("expected a non-nil Cause")
			}

			if testCase.wantInput != nil {
				wantInput := testCase.wantInput(dir)
				gotInput, ok := altshiftErr.Input.(string)
				if !ok {
					t.Errorf("Input = %v (%T), want string %q", altshiftErr.Input, altshiftErr.Input, wantInput)
				} else if gotInput != wantInput {
					t.Errorf("Input = %q, want %q", gotInput, wantInput)
				}
			}
		})
	}
}
