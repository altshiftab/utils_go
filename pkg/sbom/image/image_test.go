package image

import (
	"archive/tar"
	"bytes"
	"context"
	"debug/buildinfo"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type entry struct {
	name     string
	typeflag byte
	content  []byte
	linkname string
}

func regular(name string, content string) entry {
	return entry{name: name, typeflag: tar.TypeReg, content: []byte(content)}
}

func regularBytes(name string, content []byte) entry {
	return entry{name: name, typeflag: tar.TypeReg, content: content}
}

func link(name, target string) entry {
	return entry{name: name, typeflag: tar.TypeSymlink, linkname: target}
}

func writeTar(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for _, e := range entries {
		header := &tar.Header{Name: e.name, Typeflag: e.typeflag, Mode: 0o644, Linkname: e.linkname, Size: int64(len(e.content))}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatalf("write header %s: %v", e.name, err)
		}
		if e.typeflag == tar.TypeReg {
			if _, err := writer.Write(e.content); err != nil {
				t.Fatalf("write %s: %v", e.name, err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	return buffer.Bytes()
}

// writeArchive writes a docker archive of the given layers (lowest first) tagged "localhost/app:latest".
func writeArchive(t *testing.T, layers ...[]entry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	write := func(name string, data []byte) {
		if err := writer.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o444, Size: int64(len(data))}); err != nil {
			t.Fatalf("write header %s: %v", name, err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	var layerNames []string
	for i, layer := range layers {
		name := "layer" + string(rune('0'+i)) + ".tar"
		write(name, writeTar(t, layer))
		layerNames = append(layerNames, `"`+name+`"`)
	}
	write("cfg0123.json", []byte(`{"architecture":"amd64"}`))
	write("manifest.json", []byte(`[{"Config":"cfg0123.json","RepoTags":["localhost/app:latest"],"Layers":[`+strings.Join(layerNames, ",")+`]}]`))
	if err := writer.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	return buffer.Bytes()
}

const alpineOsRelease = "NAME=\"Alpine Linux\"\nID=alpine\nVERSION_ID=3.24.1\nPRETTY_NAME=\"Alpine Linux v3.24\"\n"

const apkInstalled = "P:musl\nV:1.2.6-r2\nA:x86_64\no:musl\n\nP:ssl_client\nV:1.37.0-r31\nA:x86_64\no:busybox\n"

func testBinary(t *testing.T) ([]byte, *buildinfo.BuildInfo) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os executable: %v", err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatalf("read executable: %v", err)
	}
	info, err := buildinfo.ReadFile(executable)
	if err != nil {
		t.Fatalf("read build info: %v", err)
	}
	return data, info
}

func TestAnalyzeArchive(t *testing.T) {
	t.Parallel()

	binary, binaryInfo := testBinary(t)

	testCases := []struct {
		name   string
		layers [][]entry
		check  func(t *testing.T, analysis *Analysis)
	}{
		{
			name: "alpine image with os-release symlink, apk database and a Go binary",
			layers: [][]entry{
				{
					link("etc/os-release", "../usr/lib/os-release"),
					regular("usr/lib/os-release", alpineOsRelease),
					regular("lib/apk/db/installed", apkInstalled),
					regularBytes("usr/local/bin/app", binary),
					regular("usr/local/bin/script", "#!/bin/sh\necho hi"),
					regularBytes("usr/lib/libfake.so", append([]byte("\x7fELF"), bytes.Repeat([]byte{0}, 100)...)),
				},
			},
			check: func(t *testing.T, analysis *Analysis) {
				t.Helper()
				if analysis.OsRelease == nil || analysis.OsRelease.Id != "alpine" || analysis.OsRelease.VersionId != "3.24.1" || analysis.OsReleasePath != "usr/lib/os-release" {
					t.Errorf("unexpected os-release: %+v at %q", analysis.OsRelease, analysis.OsReleasePath)
				}
				if len(analysis.ApkPackages) != 2 || analysis.ApkPackages[1].Origin != "busybox" || analysis.ApkDatabasePath != "lib/apk/db/installed" {
					t.Errorf("unexpected apk packages: %+v at %q", analysis.ApkPackages, analysis.ApkDatabasePath)
				}
				if len(analysis.GoBinaries) != 1 || analysis.GoBinaries[0].Path != "usr/local/bin/app" {
					t.Fatalf("expected the Go binary, got %+v", analysis.GoBinaries)
				}
				if got := analysis.GoBinaries[0].Info; got.GoVersion != binaryInfo.GoVersion || got.Path != binaryInfo.Path || len(got.Deps) != len(binaryInfo.Deps) {
					t.Errorf("build info differs: got %s %s %d deps, want %s %s %d deps", got.GoVersion, got.Path, len(got.Deps), binaryInfo.GoVersion, binaryInfo.Path, len(binaryInfo.Deps))
				}
				if len(analysis.Warnings) != 0 {
					t.Errorf("unexpected warnings: %v", analysis.Warnings)
				}
			},
		},
		{
			name: "etc/os-release wins over usr/lib/os-release; lib apk db wins over usr/lib",
			layers: [][]entry{
				{
					regular("usr/lib/os-release", "ID=wrong\nVERSION_ID=1\n"),
					regular("etc/os-release", "ID=right\nVERSION_ID=2\n"),
					regular("usr/lib/apk/db/installed", "P:wrong\nV:1-r0\n"),
					regular("lib/apk/db/installed", "P:right\nV:2-r0\n"),
				},
			},
			check: func(t *testing.T, analysis *Analysis) {
				t.Helper()
				if analysis.OsRelease.Id != "right" || analysis.OsReleasePath != "etc/os-release" {
					t.Errorf("unexpected os-release: %+v at %q", analysis.OsRelease, analysis.OsReleasePath)
				}
				if len(analysis.ApkPackages) != 1 || analysis.ApkPackages[0].Name != "right" {
					t.Errorf("unexpected apk packages: %+v", analysis.ApkPackages)
				}
			},
		},
		{
			name: "upper layer replaces the apk database and removes the binary",
			layers: [][]entry{
				{
					regular("etc/os-release", alpineOsRelease),
					regular("lib/apk/db/installed", "P:old\nV:1-r0\nA:x86_64\n"),
					regularBytes("usr/local/bin/app", binary),
				},
				{
					regular("lib/apk/db/installed", "P:new\nV:2-r0\nA:x86_64\n\nP:clamav\nV:1.4.3-r0\nA:x86_64\n"),
					regular("usr/local/bin/.wh.app", ""),
				},
			},
			check: func(t *testing.T, analysis *Analysis) {
				t.Helper()
				var names []string
				for _, p := range analysis.ApkPackages {
					names = append(names, p.Name)
				}
				if !slices.Equal(names, []string{"new", "clamav"}) {
					t.Errorf("unexpected apk packages: %v", names)
				}
				if len(analysis.GoBinaries) != 0 {
					t.Errorf("expected the whited-out binary to be gone, got %+v", analysis.GoBinaries)
				}
			},
		},
		{
			name: "debian image with status and status.d, node packages, rpm marker",
			layers: [][]entry{
				{
					regular("etc/os-release", "ID=debian\nVERSION_ID=\"13\"\n"),
					regular("var/lib/dpkg/status", "Package: bsdutils\nStatus: install ok installed\nArchitecture: amd64\nSource: util-linux (2.41-5)\nVersion: 1:2.41-5\n\nPackage: gone\nStatus: deinstall ok config-files\nVersion: 1\n"),
					regular("var/lib/dpkg/status.d/base-files", "Package: base-files\nVersion: 13.8\nArchitecture: amd64\n"),
					regular("var/lib/dpkg/status.d/base-files.md5sums", "abc  usr/share/x\n"),
					regular("app/node_modules/lit/package.json", `{"name": "lit", "version": "3.3.0", "license": "BSD-3-Clause"}`),
					regular("app/node_modules/@lit/reactive-element/package.json", `{"name": "@lit/reactive-element", "version": "2.1.0", "license": {"type": "MIT"}}`),
					regular("app/node_modules/lit/node_modules/nested/package.json", `{"name": "nested", "version": "0.1.0"}`),
					regular("app/node_modules/lit/src/package.json", `{"name": "too-deep", "version": "1"}`),
					regular("app/node_modules/broken/package.json", `{"name": "broken"}`),
					regular("app/package.json", `{"name": "app", "version": "1.0.0"}`),
					regular("var/lib/rpm/rpmdb.sqlite", "SQLite format 3\x00"),
				},
			},
			check: func(t *testing.T, analysis *Analysis) {
				t.Helper()
				if len(analysis.DpkgPackages) != 2 || analysis.DpkgPackages[0].Name != "bsdutils" || analysis.DpkgPackages[0].SourceName != "util-linux" || analysis.DpkgPackages[1].Name != "base-files" {
					t.Errorf("unexpected dpkg packages: %+v", analysis.DpkgPackages)
				}
				if !slices.Equal(analysis.DpkgStatusPaths, []string{"var/lib/dpkg/status", "var/lib/dpkg/status.d/base-files"}) {
					t.Errorf("unexpected dpkg paths: %v", analysis.DpkgStatusPaths)
				}
				var names []string
				for _, p := range analysis.NodePackages {
					names = append(names, p.Name+"@"+p.Version+"/"+p.License)
				}
				if !slices.Equal(names, []string{"@lit/reactive-element@2.1.0/MIT", "nested@0.1.0/", "lit@3.3.0/BSD-3-Clause"}) {
					t.Errorf("unexpected node packages: %v", names)
				}
				if len(analysis.Warnings) != 1 || !strings.Contains(analysis.Warnings[0], "rpm database present at /var/lib/rpm/rpmdb.sqlite") {
					t.Errorf("expected the rpm warning, got %v", analysis.Warnings)
				}
			},
		},
		{
			name: "packages without os-release warn",
			layers: [][]entry{
				{regular("lib/apk/db/installed", "P:musl\nV:1.2.6-r2\n")},
			},
			check: func(t *testing.T, analysis *Analysis) {
				t.Helper()
				if analysis.OsRelease != nil || len(analysis.Warnings) != 1 {
					t.Errorf("expected a missing os-release warning, got %+v %v", analysis.OsRelease, analysis.Warnings)
				}
			},
		},
		{
			name:   "scratch-like image with nothing",
			layers: [][]entry{{regular("etc/ssl/certs/ca-certificates.crt", "PEM")}},
			check: func(t *testing.T, analysis *Analysis) {
				t.Helper()
				if analysis.OsRelease != nil || len(analysis.ApkPackages) != 0 || len(analysis.DpkgPackages) != 0 || len(analysis.GoBinaries) != 0 || len(analysis.NodePackages) != 0 || len(analysis.Warnings) != 0 {
					t.Errorf("expected an empty analysis, got %+v", analysis)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			analysis, err := AnalyzeArchive(bytes.NewReader(writeArchive(t, testCase.layers...)), "localhost/app:latest")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if analysis.Id != "sha256:cfg0123" || analysis.Reference != "localhost/app:latest" || !slices.Equal(analysis.RepoTags, []string{"localhost/app:latest"}) {
				t.Errorf("unexpected identity: %+v", analysis)
			}
			testCase.check(t, analysis)
		})
	}
}

func TestClassify(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path     string
		expected fileKind
	}{
		{path: "etc/os-release", expected: kindOsRelease},
		{path: "usr/lib/os-release", expected: kindOsRelease},
		{path: "lib/apk/db/installed", expected: kindApkDatabase},
		{path: "var/lib/dpkg/status", expected: kindDpkgStatus},
		{path: "var/lib/dpkg/status.d/libc6", expected: kindDpkgStatus},
		{path: "var/lib/dpkg/status.d/libc6.md5sums", expected: kindOther},
		{path: "var/lib/dpkg/status.d/sub/deep", expected: kindOther},
		{path: "var/lib/dpkg/status-old", expected: kindOther},
		{path: "var/lib/rpm/Packages", expected: kindRpmDatabase},
		{path: "usr/lib/sysimage/rpm/rpmdb.sqlite", expected: kindRpmDatabase},
		{path: "node_modules/express/package.json", expected: kindNodePackage},
		{path: "app/node_modules/@types/node/package.json", expected: kindNodePackage},
		{path: "app/node_modules/express/node_modules/debug/package.json", expected: kindNodePackage},
		{path: "app/node_modules/express/lib/package.json", expected: kindOther},
		{path: "app/node_modules/@types/package.json", expected: kindOther},
		{path: "app/node_modules/.bin/package.json", expected: kindOther},
		{path: "app/package.json", expected: kindOther},
		{path: "usr/bin/ls", expected: kindOther},
	}

	for _, testCase := range testCases {
		t.Run(testCase.path, func(t *testing.T) {
			t.Parallel()

			if got := classify(testCase.path); got != testCase.expected {
				t.Errorf("expected %d, got %d", testCase.expected, got)
			}
		})
	}
}

// writeFakePodman writes a script that behaves like `podman save`: it writes the fixture to stdout, or fails.
func writeFakePodman(t *testing.T, fixture []byte, fail bool) string {
	t.Helper()
	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "image.tar")
	if err := os.WriteFile(fixturePath, fixture, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	script := "#!/bin/sh\n"
	if fail {
		script += "echo 'Error: localhost/app:latest: image not known' >&2\nexit 125\n"
	} else {
		script += "[ \"$1\" = save ] && [ \"$2\" = --format ] && [ \"$3\" = docker-archive ] && [ \"$4\" = -- ] || { echo bad args >&2; exit 2; }\n"
		script += "cat '" + fixturePath + "'\n"
	}
	scriptPath := filepath.Join(dir, "podman")
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil { //nolint:gosec // executable test script
		t.Fatalf("write script: %v", err)
	}
	return scriptPath
}

func TestStoreAnalyze(t *testing.T) {
	t.Parallel()

	archive := writeArchive(t, []entry{regular("etc/os-release", alpineOsRelease), regular("lib/apk/db/installed", apkInstalled)})

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		store := &Store{Podman: writeFakePodman(t, archive, false)}
		analysis, err := store.Analyze(context.Background(), "localhost/app:latest")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if analysis.OsRelease == nil || analysis.OsRelease.Id != "alpine" || len(analysis.ApkPackages) != 2 {
			t.Errorf("unexpected analysis: %+v", analysis)
		}
	})

	t.Run("podman failure is reported", func(t *testing.T) {
		t.Parallel()

		store := &Store{Podman: writeFakePodman(t, nil, true)}
		_, err := store.Analyze(context.Background(), "localhost/app:latest")
		if !errors.Is(err, ErrPodmanFailed) || !strings.Contains(err.Error(), "image not known") {
			t.Fatalf("expected the podman failure with its message, got %v", err)
		}
	})

	t.Run("podman missing", func(t *testing.T) {
		t.Parallel()

		store := &Store{Podman: filepath.Join(t.TempDir(), "no-such-podman")}
		_, err := store.Analyze(context.Background(), "localhost/app:latest")
		if err == nil {
			t.Fatalf("expected an error")
		}
	})

	t.Run("empty reference", func(t *testing.T) {
		t.Parallel()

		if _, err := (&Store{}).Analyze(context.Background(), ""); err == nil {
			t.Fatalf("expected an error")
		}
	})
}

func TestStoreAnalyzeRealPodman(t *testing.T) {
	t.Parallel()

	if _, err := os.Stat("/usr/bin/podman"); err != nil {
		t.Skip("podman not installed")
	}
	if err := execCommand(t, "podman", "image", "exists", "docker.io/library/alpine:3.24"); err != nil {
		t.Skip("alpine:3.24 not in the local store")
	}

	analysis, err := (&Store{}).Analyze(context.Background(), "docker.io/library/alpine:3.24")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if analysis.OsRelease == nil || analysis.OsRelease.Id != "alpine" || len(analysis.ApkPackages) < 10 {
		t.Errorf("unexpected analysis of alpine: %+v", analysis)
	}
}
