package sbom

import (
	"archive/tar"
	"bytes"
	"cmp"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/sbom/image"
	altshiftSbomTypes "github.com/altshiftab/utils_go/pkg/sbom/types"
)

// dockerArchive builds a one-layer docker archive of the given files, tagged as reference.
func dockerArchive(t *testing.T, reference string, files map[string]string) []byte {
	t.Helper()
	var layer bytes.Buffer
	layerWriter := tar.NewWriter(&layer)
	for name, content := range files {
		if err := layerWriter.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatalf("write layer header: %v", err)
		}
		if _, err := layerWriter.Write([]byte(content)); err != nil {
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
	write("cfg.json", []byte(`{"rootfs":{"diff_ids":["sha256:layer0"]}}`))
	write("manifest.json", []byte(`[{"Config":"cfg.json","RepoTags":["`+reference+`"],"Layers":["layer0.tar"]}]`))
	if err := writer.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	return archive.Bytes()
}

// fakePodman writes a script serving one archive per reference.
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

func TestDescribe(t *testing.T) {
	t.Parallel()

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os executable: %v", err)
	}
	podman := fakePodman(t, map[string][]byte{
		"localhost/app:latest": dockerArchive(t, "localhost/app:latest", map[string]string{"etc/os-release": "ID=alpine\nVERSION_ID=3.24.1\n", "lib/apk/db/installed": "P:musl\nV:1.2.6-r2\nA:x86_64\n"}),
		"golang:1.26-alpine":   dockerArchive(t, "docker.io/library/golang:1.26-alpine", map[string]string{"etc/os-release": "ID=alpine\nVERSION_ID=3.24.1\n", "var/lib/rpm/Packages": "x"}),
	})
	store := &image.Store{Podman: podman}

	testCases := []struct {
		name       string
		sources    *Sources
		subject    string
		components []string
		warnings   []string
		err        bool
	}{
		{
			name:    "image, dockerfile with build stage, node lock and go binary",
			sources: &Sources{Image: "localhost/app:latest", Dockerfile: []byte("FROM golang:1.26-alpine AS builder\nFROM ${BASE}\nFROM scratch\n"), NodeLock: []byte(`{"packages":{"node_modules/lit":{"version":"3.3.0"},"node_modules/typescript":{"version":"5.9.2","dev":true}}}`), GoBinaries: []string{executable}},
			subject: "pkg:docker/localhost/app@latest",
			components: []string{
				"os:alpine@3.24.1 required",
				"pkg:apk/alpine/musl@1.2.6-r2?arch=x86_64&distro=alpine-3.24.1 required",
				"pkg:npm/lit@3.3.0 required",
				"pkg:npm/typescript@5.9.2 excluded",
				"pkg:docker/golang@1.26-alpine excluded",
				"pkg:docker/$%7BBASE%7D excluded",
			},
			warnings: []string{"golang:1.26-alpine: rpm database present at /var/lib/rpm/Packages; RPM packages are not listed", "${BASE}: image reference holds a build argument; its contents are not listed"},
		},
		{
			name:       "final base only needs no image",
			sources:    &Sources{Dockerfile: []byte("FROM alpine:3.24\n")},
			components: []string{"pkg:docker/alpine@3.24 required"},
		},
		{
			name:    "unknown image",
			sources: &Sources{Image: "localhost/nope:latest"},
			err:     true,
		},
		{
			name:    "missing go binary",
			sources: &Sources{GoBinaries: []string{filepath.Join(t.TempDir(), "missing")}},
			err:     true,
		},
		{
			name: "nil sources",
			err:  true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			description, err := Describe(context.Background(), store, testCase.sources)
			if testCase.err {
				if err == nil {
					t.Fatalf("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var subject string
			if description.Subject != nil {
				subject = description.Subject.Purl
			}
			if subject != testCase.subject {
				t.Errorf("expected subject %q, got %q", testCase.subject, subject)
			}
			got := make(map[string]bool)
			for _, component := range description.Components {
				got[cmp.Or(component.Purl, component.BomRef)+" "+string(component.Scope)] = true
			}
			for _, expected := range testCase.components {
				if !got[expected] {
					t.Errorf("missing component %q in %v", expected, keys(got))
				}
			}
			// The Go binary's own components come along when it is given.
			if len(testCase.sources.GoBinaries) != 0 {
				var stdlib bool
				for _, component := range description.Components {
					stdlib = stdlib || (component.Name == "stdlib" && component.Scope == altshiftSbomTypes.ScopeRequired)
				}
				if !stdlib {
					t.Errorf("expected the go binary's stdlib component")
				}
			}
			for _, warning := range testCase.warnings {
				var found bool
				for _, got := range description.Warnings {
					found = found || strings.HasPrefix(got, warning)
				}
				if !found {
					t.Errorf("missing warning %q in %v", warning, description.Warnings)
				}
			}
		})
	}
}

func TestDescribeJson(t *testing.T) {
	t.Parallel()

	data, warnings, err := DescribeJson(context.Background(), nil, &Sources{Dockerfile: []byte("FROM alpine:3.24\n")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 || !strings.Contains(string(data), `"purl": "pkg:docker/alpine@3.24"`) {
		t.Errorf("unexpected result: %v %s", warnings, data)
	}
}

func keys(set map[string]bool) []string {
	var result []string
	for key := range set {
		result = append(result, key)
	}
	return result
}
