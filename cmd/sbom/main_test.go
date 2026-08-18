package main

import (
	"archive/tar"
	"bytes"
	"context"
	"debug/buildinfo"
	"encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	altshiftSbomTypes "github.com/altshiftab/utils_go/pkg/sbom/types"
)

// dockerArchive builds a one-layer docker archive of the given files, tagged as reference.
func dockerArchive(t *testing.T, reference string, files map[string][]byte) []byte {
	t.Helper()

	var layer bytes.Buffer
	layerWriter := tar.NewWriter(&layer)
	for name, content := range files {
		if err := layerWriter.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatalf("write layer header: %v", err)
		}
		if _, err := layerWriter.Write(content); err != nil {
			t.Fatalf("write layer file: %v", err)
		}
	}
	if err := layerWriter.Close(); err != nil {
		t.Fatalf("close layer: %v", err)
	}

	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	write := func(name string, data []byte) {
		if err := writer.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o444, Size: int64(len(data))}); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	write("layer0.tar", layer.Bytes())
	write("cfg.json", []byte(`{}`))
	write("manifest.json", []byte(`[{"Config":"cfg.json","RepoTags":["`+reference+`"],"Layers":["layer0.tar"]}]`))
	if err := writer.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	return archive.Bytes()
}

// fakePodman writes a script that serves one archive per reference and fails for any other.
func fakePodman(t *testing.T, archives map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$5\" in\n"
	i := 0
	for reference, archive := range archives {
		fixture := filepath.Join(dir, "image"+string(rune('a'+i))+".tar")
		i++
		if err := os.WriteFile(fixture, archive, 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		script += "  '" + reference + "') cat '" + fixture + "' ;;\n"
	}
	script += "  *) echo \"Error: $5: image not known\" >&2; exit 125 ;;\nesac\n"
	scriptPath := filepath.Join(dir, "podman")
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil { //nolint:gosec // executable test script
		t.Fatalf("write script: %v", err)
	}
	return scriptPath
}

// runCommand runs the command and returns its exit code, error, the BOM it wrote to stdout (an empty BOM when it wrote
// nothing) and its stderr.
func runCommand(t *testing.T, args ...string) (int, *altshiftSbomTypes.Bom, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code, err := run(context.Background(), args, &stdout, &stderr)
	bom := &altshiftSbomTypes.Bom{}
	if stdout.Len() != 0 {
		if unmarshalErr := json.Unmarshal(stdout.Bytes(), bom); unmarshalErr != nil {
			t.Fatalf("stdout is not a bom: %v: %s", unmarshalErr, stdout.String())
		}
	}
	return code, bom, stderr.String(), err
}

func componentRefs(components []*altshiftSbomTypes.Component) map[string]*altshiftSbomTypes.Component {
	byRef := make(map[string]*altshiftSbomTypes.Component)
	for _, component := range components {
		byRef[component.BomRef] = component
	}
	return byRef
}

const alpineOsRelease = "ID=alpine\nVERSION_ID=3.24.1\n"

func TestRunUsage(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		args         []string
		expectedCode int
		stderr       string
	}{
		{name: "help", args: []string{"--help"}, expectedCode: exitClean},
		{name: "old single-dash spelling is rejected", args: []string{"-go", "go.sum"}, expectedCode: exitUsage, stderr: "error"},
		{name: "unknown option", args: []string{"--nope"}, expectedCode: exitUsage, stderr: "error"},
		{name: "nothing to describe", args: nil, expectedCode: exitUsage, stderr: "nothing to describe"},
		{name: "abbreviations are disabled", args: []string{"--out", "x"}, expectedCode: exitUsage, stderr: "error"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			code, _, stderr, err := runCommand(t, testCase.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if code != testCase.expectedCode {
				t.Errorf("expected exit %d, got %d (stderr %q)", testCase.expectedCode, code, stderr)
			}
			if testCase.stderr != "" && !strings.Contains(stderr, testCase.stderr) {
				t.Errorf("expected stderr to mention %q, got %q", testCase.stderr, stderr)
			}
		})
	}
}

func TestRunGoBinary(t *testing.T) {
	t.Parallel()

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os executable: %v", err)
	}
	info, err := buildinfo.ReadFile(executable)
	if err != nil {
		t.Fatalf("read build info: %v", err)
	}

	code, bom, _, err := runCommand(t, "--go-binary", executable)
	if err != nil || code != exitClean {
		t.Fatalf("unexpected result: %d %v", code, err)
	}
	if bom.Metadata == nil || bom.Metadata.Component != nil {
		t.Errorf("expected no subject without --image, got %+v", bom.Metadata)
	}
	byRef := componentRefs(bom.Components)
	stdlib, ok := byRef["pkg:golang/stdlib@"+strings.TrimPrefix(strings.Fields(info.GoVersion)[0], "go")]
	if !ok || stdlib == nil {
		t.Fatalf("expected the stdlib component, got %v", refsOf(bom.Components))
	}
	if stdlib.Scope != altshiftSbomTypes.ScopeRequired || len(stdlib.Properties) != 1 || stdlib.Properties[0].Value != executable {
		t.Errorf("unexpected stdlib component: %+v", stdlib)
	}
	// Every linked module is listed, and nothing else.
	if len(bom.Components) != len(info.Deps)+2 {
		t.Errorf("expected main + stdlib + %d deps, got %d components", len(info.Deps), len(bom.Components))
	}
}

func TestRunNode(t *testing.T) {
	t.Parallel()

	lock := filepath.Join(t.TempDir(), "package-lock.json")
	if err := os.WriteFile(lock, []byte(`{"name":"app","version":"1.0.0","lockfileVersion":3,"packages":{"":{"name":"app","version":"1.0.0"},"node_modules/lit":{"version":"3.3.0"},"node_modules/typescript":{"version":"5.9.2","dev":true}}}`), 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	code, bom, _, err := runCommand(t, "--node", lock)
	if err != nil || code != exitClean {
		t.Fatalf("unexpected result: %d %v", code, err)
	}
	byRef := componentRefs(bom.Components)
	if lit := byRef["pkg:npm/lit@3.3.0"]; lit == nil || lit.Scope != altshiftSbomTypes.ScopeRequired {
		t.Errorf("expected lit as required, got %+v", lit)
	}
	if typescript := byRef["pkg:npm/typescript@5.9.2"]; typescript == nil || typescript.Scope != altshiftSbomTypes.ScopeExcluded {
		t.Errorf("expected typescript as excluded, got %+v", typescript)
	}
}

func TestRunImageAndDockerfile(t *testing.T) {
	t.Parallel()

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os executable: %v", err)
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatalf("read executable: %v", err)
	}

	podman := fakePodman(t, map[string][]byte{
		"localhost/app:latest": dockerArchive(t, "localhost/app:latest", map[string][]byte{
			"etc/ssl/certs/ca-certificates.crt": []byte("PEM"),
			"app":                               binary,
		}),
		"golang:1.26-alpine": dockerArchive(t, "docker.io/library/golang:1.26-alpine", map[string][]byte{
			"etc/os-release":       []byte(alpineOsRelease),
			"lib/apk/db/installed": []byte("P:musl\nV:1.2.6-r2\nA:x86_64\no:musl\n\nP:ssl_client\nV:1.37.0-r31\nA:x86_64\no:busybox\n"),
			"var/lib/rpm/Packages": []byte("bdb"),
		}),
	})
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM golang:1.26-alpine AS builder\nRUN go build -o /app\nFROM scratch\nCOPY --from=builder /app /app\n"), 0o600); err != nil {
		t.Fatalf("write dockerfile: %v", err)
	}
	output := filepath.Join(dir, "sbom.json")

	code, _, stderr, err := runCommand(t, "--podman", podman, "--image", "localhost/app:latest", "--dockerfile", dockerfile, "--output", output)
	if err != nil || code != exitClean {
		t.Fatalf("unexpected result: %d %v (%s)", code, err, stderr)
	}
	if !strings.Contains(stderr, "rpm database present") {
		t.Errorf("expected the rpm warning on stderr, got %q", stderr)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var bom altshiftSbomTypes.Bom
	if err := json.Unmarshal(data, &bom); err != nil {
		t.Fatalf("output is not a bom: %v", err)
	}

	if bom.Metadata == nil {
		t.Fatalf("expected metadata")
	}
	subject := bom.Metadata.Component
	if subject == nil || subject.Type != altshiftSbomTypes.ComponentTypeContainer || subject.Purl != "pkg:docker/localhost/app@latest" || len(subject.Properties) != 2 || subject.Properties[0].Value != "localhost/app:latest" || subject.Properties[1].Value != "sha256:cfg" {
		t.Errorf("unexpected subject: %+v", subject)
	}

	byRef := componentRefs(bom.Components)
	// The runtime image holds the Go binary: its stdlib is required and points at /app.
	var runtimeStdlib *altshiftSbomTypes.Component
	for _, component := range bom.Components {
		if component.Name == "stdlib" {
			runtimeStdlib = component
		}
	}
	if runtimeStdlib == nil || runtimeStdlib.Scope != altshiftSbomTypes.ScopeRequired || len(runtimeStdlib.Properties) != 2 || runtimeStdlib.Properties[1].Value != "/app" {
		t.Errorf("unexpected runtime stdlib component: %+v", runtimeStdlib)
	}
	// The builder image is an excluded container holding its packages, nested with prefixed bom-refs.
	builder := byRef["pkg:docker/golang@1.26-alpine"]
	if builder == nil || builder.Scope != altshiftSbomTypes.ScopeExcluded || len(builder.Components) != 3 {
		t.Fatalf("unexpected builder component: %+v", builder)
	}
	nested := componentRefs(builder.Components)
	sslClient := nested["pkg:docker/golang@1.26-alpine/pkg:apk/alpine/ssl_client@1.37.0-r31?arch=x86_64&distro=alpine-3.24.1&upstream=busybox"]
	if sslClient == nil || sslClient.Scope != altshiftSbomTypes.ScopeExcluded || sslClient.Purl != "pkg:apk/alpine/ssl_client@1.37.0-r31?arch=x86_64&distro=alpine-3.24.1&upstream=busybox" {
		t.Errorf("unexpected nested apk component: %v", refsOf(builder.Components))
	}
	if osComponent := nested["pkg:docker/golang@1.26-alpine/os:alpine@3.24.1"]; osComponent == nil || osComponent.Type != altshiftSbomTypes.ComponentTypeOperatingSystem {
		t.Errorf("expected the nested os component, got %v", refsOf(builder.Components))
	}
	// scratch is not a component.
	for ref := range byRef {
		if strings.Contains(ref, "scratch") {
			t.Errorf("unexpected scratch component %s", ref)
		}
	}
}

func TestRunDockerfileFinalBaseOnly(t *testing.T) {
	t.Parallel()

	dockerfile := filepath.Join(t.TempDir(), "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM alpine:3.24\nRUN apk add curl\n"), 0o600); err != nil {
		t.Fatalf("write dockerfile: %v", err)
	}

	// No build stages, so podman is never needed.
	code, bom, _, err := runCommand(t, "--podman", "/nonexistent/podman", "--dockerfile", dockerfile)
	if err != nil || code != exitClean {
		t.Fatalf("unexpected result: %d %v", code, err)
	}
	if len(bom.Components) != 1 || bom.Components[0] == nil {
		t.Fatalf("expected only the final base, got %+v", bom.Components)
	}
	if base := bom.Components[0]; base.Purl != "pkg:docker/alpine@3.24" || base.Scope != altshiftSbomTypes.ScopeRequired || len(base.Components) != 0 {
		t.Errorf("expected the final base as a required container, got %+v", base)
	}
}

func TestRunErrors(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing")
	podman := fakePodman(t, nil)

	testCases := []struct {
		name string
		args []string
	}{
		{name: "missing go binary", args: []string{"--go-binary", missing}},
		{name: "not a go binary", args: []string{"--go-binary", "/etc/hostname"}},
		{name: "missing package-lock", args: []string{"--node", missing}},
		{name: "missing dockerfile", args: []string{"--dockerfile", missing}},
		{name: "unknown image", args: []string{"--podman", podman, "--image", "localhost/nope:latest"}},
		{name: "unwritable output", args: []string{"--node", "/dev/null", "--output", filepath.Join(missing, "sub", "sbom.json")}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			code, _, _, err := runCommand(t, testCase.args...)
			if err == nil || code != exitError {
				t.Errorf("expected exit %d with an error, got %d %v", exitError, code, err)
			}
		})
	}
}

func refsOf(components []*altshiftSbomTypes.Component) []string {
	var result []string
	for _, component := range components {
		result = append(result, component.BomRef)
	}
	return result
}
