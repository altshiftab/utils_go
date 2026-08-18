package sbom

import (
	"debug/buildinfo"
	"runtime/debug"
	"slices"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/sbom/image"
	"github.com/altshiftab/utils_go/pkg/sbom/ospkg"
	altshiftSbomTypes "github.com/altshiftab/utils_go/pkg/sbom/types"
)

func TestEscapePurlSegment(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		input    string
		expected string
	}{
		{input: "1.2.3", expected: "1.2.3"},
		{input: "v1.2.3+incompatible", expected: "v1.2.3%2Bincompatible"},
		{input: "3.0.11-1~deb12u2", expected: "3.0.11-1~deb12u2"},
		{input: "2.41-5+deb13u1", expected: "2.41-5%2Bdeb13u1"},
		{input: "a&b=c", expected: "a%26b%3Dc"},
		{input: "name@1.0", expected: "name%401.0"},
		{input: "with space", expected: "with%20space"},
		{input: "path/like", expected: "path%2Flike"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.input, func(t *testing.T) {
			t.Parallel()

			if got := escapePurlSegment(testCase.input); got != testCase.expected {
				t.Errorf("expected %q, got %q", testCase.expected, got)
			}
		})
	}
}

func TestOsPackagePurl(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		purlType   string
		namespace  string
		pkgName    string
		version    string
		qualifiers map[string]string
		expected   string
	}{
		{
			name: "apk with upstream", purlType: "apk", namespace: "alpine", pkgName: "ssl_client", version: "1.37.0-r31",
			qualifiers: map[string]string{"upstream": "busybox", "distro": "alpine-3.24.1", "arch": "x86_64"},
			expected:   "pkg:apk/alpine/ssl_client@1.37.0-r31?arch=x86_64&distro=alpine-3.24.1&upstream=busybox",
		},
		{
			name: "empty qualifiers dropped", purlType: "deb", namespace: "debian", pkgName: "bash", version: "5.2.37-2",
			qualifiers: map[string]string{"arch": "", "distro": "debian-13"},
			expected:   "pkg:deb/debian/bash@5.2.37-2?distro=debian-13",
		},
		{
			name: "no version, no qualifiers", purlType: "apk", namespace: "alpine", pkgName: "musl",
			expected: "pkg:apk/alpine/musl",
		},
		{
			name: "escaped values", purlType: "deb", namespace: "debian", pkgName: "libgcc-s1", version: "14.2.0-19+b1",
			qualifiers: map[string]string{"upstream": "gcc-14@14.2.0-19+b1"},
			expected:   "pkg:deb/debian/libgcc-s1@14.2.0-19%2Bb1?upstream=gcc-14%4014.2.0-19%2Bb1",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := osPackagePurl(testCase.purlType, testCase.namespace, testCase.pkgName, testCase.version, testCase.qualifiers); got != testCase.expected {
				t.Errorf("expected %q, got %q", testCase.expected, got)
			}
		})
	}
}

func TestSplitImageReference(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		image   string
		name    string
		version string
	}{
		{image: "golang:1.26-alpine", name: "golang", version: "1.26-alpine"},
		{image: "docker.io/library/alpine:3.24", name: "docker.io/library/alpine", version: "3.24"},
		{image: "localhost:5000/app:latest", name: "localhost:5000/app", version: "latest"},
		{image: "localhost:5000/app", name: "localhost:5000/app", version: ""},
		{image: "alpine@sha256:abc", name: "alpine", version: "sha256:abc"},
		{image: "alpine", name: "alpine", version: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.image, func(t *testing.T) {
			t.Parallel()

			name, version := splitImageReference(testCase.image)
			if name != testCase.name || version != testCase.version {
				t.Errorf("expected (%q, %q), got (%q, %q)", testCase.name, testCase.version, name, version)
			}
		})
	}
}

func TestParseDockerfileStagesAndImages(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		dockerfile  string
		stages      []*Stage
		buildImages []string
		finalBase   string
	}{
		{
			name:        "builder and scratch",
			dockerfile:  "FROM golang:1.26-alpine AS builder\nRUN go build\n\nFROM scratch\nCOPY --from=builder /app /app\n",
			stages:      []*Stage{{Image: "golang:1.26-alpine", Alias: "builder"}, {Image: "scratch"}},
			buildImages: []string{"golang:1.26-alpine"},
			finalBase:   "",
		},
		{
			name:        "helper stage, final alpine, platform, lower case",
			dockerfile:  "from --platform=linux/amd64 docker.io/golang:1.26-alpine as Builder\nFROM docker.io/clamav/clamav:latest AS clamdb\nFROM docker.io/alpine:3.24\n",
			stages:      []*Stage{{Image: "docker.io/golang:1.26-alpine", Alias: "Builder", Platform: "linux/amd64"}, {Image: "docker.io/clamav/clamav:latest", Alias: "clamdb"}, {Image: "docker.io/alpine:3.24"}},
			buildImages: []string{"docker.io/golang:1.26-alpine", "docker.io/clamav/clamav:latest"},
			finalBase:   "docker.io/alpine:3.24",
		},
		{
			name:        "stage reference is not an image; final stage built on an alias",
			dockerfile:  "FROM alpine:3.24 AS base\nFROM base AS build\nRUN apk add x\nFROM BASE\n",
			stages:      []*Stage{{Image: "alpine:3.24", Alias: "base"}, {Image: "base", Alias: "build"}, {Image: "BASE"}},
			buildImages: nil,
			finalBase:   "alpine:3.24",
		},
		{
			name:        "same image as builder and final base is runtime only",
			dockerfile:  "FROM alpine AS b\nFROM alpine\n",
			stages:      []*Stage{{Image: "alpine", Alias: "b"}, {Image: "alpine"}},
			buildImages: nil,
			finalBase:   "alpine",
		},
		{
			name:       "empty",
			dockerfile: "",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			stages, err := ParseDockerfileStages([]byte(testCase.dockerfile))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(stages) != len(testCase.stages) {
				t.Fatalf("expected %d stages, got %+v", len(testCase.stages), stages)
			}
			for i, expected := range testCase.stages {
				if *stages[i] != *expected {
					t.Errorf("stage %d: expected %+v, got %+v", i, expected, stages[i])
				}
			}
			buildImages, finalBase := DockerfileImages(stages)
			if !slices.Equal(buildImages, testCase.buildImages) || finalBase != testCase.finalBase {
				t.Errorf("expected build images %v and final base %q, got %v and %q", testCase.buildImages, testCase.finalBase, buildImages, finalBase)
			}
		})
	}
}

func TestParseDockerfileSkipsStageReferences(t *testing.T) {
	t.Parallel()

	components, err := ParseDockerfile([]byte("FROM alpine:3.24 AS base\nFROM base\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(components) != 1 || components[0].Purl != "pkg:docker/alpine@3.24" {
		t.Errorf("expected only the real image, got %+v", components)
	}
}

func TestBuildInfoComponents(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		info     *buildinfo.BuildInfo
		expected []string
	}{
		{
			name: "application with deps, replacement and local replacement",
			info: &debug.BuildInfo{
				GoVersion: "go1.26.6 X:nodwarf5",
				Path:      "github.com/intilgroup/signals_login/backend",
				Main:      debug.Module{Path: "github.com/intilgroup/signals_login/backend", Version: "(devel)"},
				Deps: []*debug.Module{
					{Path: "golang.org/x/text", Version: "v0.37.0"},
					{Path: "github.com/old/mod", Version: "v1.0.0", Replace: &debug.Module{Path: "github.com/new/mod", Version: "v1.2.3+incompatible"}},
					{Path: "github.com/local/mod", Version: "v0.0.0", Replace: &debug.Module{Path: "../local", Version: "(devel)"}},
					nil,
				},
			},
			expected: []string{
				"application github.com/intilgroup/signals_login/backend  pkg:golang/github.com/intilgroup/signals_login/backend",
				"library stdlib 1.26.6 pkg:golang/stdlib@1.26.6",
				"library golang.org/x/text v0.37.0 pkg:golang/golang.org/x/text@v0.37.0",
				"library github.com/new/mod v1.2.3+incompatible pkg:golang/github.com/new/mod@v1.2.3%2Bincompatible",
			},
		},
		{
			name: "toolchain binary has only the stdlib",
			info: &debug.BuildInfo{GoVersion: "go1.26.6", Path: "cmd/go"},
			expected: []string{
				"library stdlib 1.26.6 pkg:golang/stdlib@1.26.6",
			},
		},
		{
			name: "experiments recorded with a dash",
			info: &debug.BuildInfo{GoVersion: "go1.26.6-X:jsonv2", Path: "cmd/go"},
			expected: []string{
				"library stdlib 1.26.6 pkg:golang/stdlib@1.26.6",
			},
		},
		{
			name: "development toolchain has no stdlib version",
			info: &debug.BuildInfo{GoVersion: "devel go1.27-abcdef Mon Jan 1", Path: "cmd/go", Main: debug.Module{Path: "example.com/app", Version: "v1.0.0"}},
			expected: []string{
				"application example.com/app v1.0.0 pkg:golang/example.com/app@v1.0.0",
				"library stdlib  pkg:golang/stdlib",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			components := BuildInfoComponents(testCase.info, "/usr/local/bin/app", altshiftSbomTypes.ScopeRequired)
			var got []string
			for _, component := range components {
				got = append(got, strings.Join([]string{string(component.Type), component.Name, component.Version, component.Purl}, " "))
				if component.BomRef != component.Purl || component.Scope != altshiftSbomTypes.ScopeRequired {
					t.Errorf("unexpected bom-ref or scope: %+v", component)
				}
				if len(component.Properties) != 1 || component.Properties[0].Name != altshiftSbomTypes.PropertyPath || component.Properties[0].Value != "/usr/local/bin/app" {
					t.Errorf("expected the path property, got %+v", component.Properties)
				}
			}
			if !slices.Equal(got, testCase.expected) {
				t.Errorf("expected %q, got %q", testCase.expected, got)
			}
		})
	}
}

func TestBuildInfoComponentsNil(t *testing.T) {
	t.Parallel()

	if components := BuildInfoComponents(nil, "", altshiftSbomTypes.ScopeRequired); components != nil {
		t.Errorf("expected nil, got %+v", components)
	}
}

func TestImageComponents(t *testing.T) {
	t.Parallel()

	analysis := &image.Analysis{
		Reference:        "golang:1.26-alpine",
		Id:               "sha256:0fe4",
		RepoTags:         []string{"docker.io/library/golang:1.26-alpine"},
		Layers:           []string{"sha256:base", "sha256:top"},
		OsRelease:        &ospkg.OsRelease{Id: "alpine", VersionId: "3.24.1"},
		OsReleasePath:    "usr/lib/os-release",
		OsReleaseLayer:   "sha256:base",
		ApkPackages:      []*ospkg.ApkPackage{{Name: "musl", Version: "1.2.6-r2", Arch: "x86_64", Origin: "musl", License: "MIT"}, {Name: "ssl_client", Version: "1.37.0-r31", Arch: "x86_64", Origin: "busybox"}},
		ApkDatabasePath:  "lib/apk/db/installed",
		ApkDatabaseLayer: "sha256:top",
		DpkgStatuses: []*image.DpkgStatus{{
			Path:     "var/lib/dpkg/status",
			Layer:    "sha256:base",
			Packages: []*ospkg.DpkgPackage{{Name: "bsdutils", Version: "1:2.41-5", Architecture: "amd64", SourceName: "util-linux", SourceVersion: "2.41-5"}, {Name: "bash", Version: "5.2.37-2", Architecture: "amd64", SourceName: "bash"}},
		}},
		GoBinaries: []*image.GoBinary{
			{Path: "usr/local/go/bin/go", Layer: "sha256:top", Info: &debug.BuildInfo{GoVersion: "go1.26.6", Path: "cmd/go"}},
			{Path: "usr/local/go/bin/gofmt", Layer: "sha256:top", Info: &debug.BuildInfo{GoVersion: "go1.26.6", Path: "cmd/gofmt"}},
		},
		NodePackages: []*image.NodePackage{{Path: "app/node_modules/lit/package.json", Layer: "sha256:top", Name: "lit", Version: "3.3.0", License: "BSD-3-Clause"}},
	}

	components, err := ImageComponents(analysis, altshiftSbomTypes.ScopeExcluded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byRef := make(map[string]*altshiftSbomTypes.Component)
	for _, component := range components {
		byRef[component.BomRef] = component
		if component.Scope != altshiftSbomTypes.ScopeExcluded {
			t.Errorf("expected scope excluded on %s", component.BomRef)
		}
		if len(component.Properties) == 0 || component.Properties[0].Name != altshiftSbomTypes.PropertyImage || component.Properties[0].Value != "docker.io/library/golang:1.26-alpine" {
			t.Errorf("expected the image property first on %s, got %+v", component.BomRef, component.Properties)
		}
	}

	expected := map[string]string{
		"os:alpine@3.24.1": "/usr/lib/os-release",
		"pkg:apk/alpine/musl@1.2.6-r2?arch=x86_64&distro=alpine-3.24.1":                                       "/lib/apk/db/installed",
		"pkg:apk/alpine/ssl_client@1.37.0-r31?arch=x86_64&distro=alpine-3.24.1&upstream=busybox":              "/lib/apk/db/installed",
		"pkg:deb/alpine/bsdutils@2.41-5?arch=amd64&distro=alpine-3.24.1&epoch=1&upstream=util-linux%402.41-5": "/var/lib/dpkg/status",
		"pkg:deb/alpine/bash@5.2.37-2?arch=amd64&distro=alpine-3.24.1":                                        "/var/lib/dpkg/status",
		"pkg:golang/stdlib@1.26.6": "/usr/local/go/bin/go",
		"pkg:npm/lit@3.3.0":        "/app/node_modules/lit/package.json",
	}
	if len(components) != len(expected) {
		t.Fatalf("expected %d components, got %d: %v", len(expected), len(components), refs(components))
	}
	for bomRef, path := range expected {
		component, ok := byRef[bomRef]
		if !ok {
			t.Errorf("missing component %s in %v", bomRef, refs(components))
			continue
		}
		if len(component.Properties) < 2 || component.Properties[1].Name != altshiftSbomTypes.PropertyPath || component.Properties[1].Value != path {
			t.Errorf("%s: expected path %q, got %+v", bomRef, path, component.Properties)
		}
	}

	// Every expected component was found above, so these lookups cannot be nil.
	stdlib := byRef["pkg:golang/stdlib@1.26.6"]
	// Two toolchain binaries in the same layer merge into one stdlib component with both paths and one layer.
	if stdlib != nil && (len(stdlib.Properties) != 4 || stdlib.Properties[3].Name != altshiftSbomTypes.PropertyPath || stdlib.Properties[3].Value != "/usr/local/go/bin/gofmt") {
		var props []string
		for _, property := range stdlib.Properties {
			props = append(props, property.Name+"="+property.Value)
		}
		t.Errorf("expected the two toolchain binaries merged into one stdlib component with both paths, got %v", props)
	}
	if osComponent := byRef["os:alpine@3.24.1"]; osComponent != nil && (osComponent.Type != altshiftSbomTypes.ComponentTypeOperatingSystem || osComponent.Purl != "" || osComponent.Name != "alpine" || osComponent.Version != "3.24.1") {
		t.Errorf("unexpected os component: %+v", osComponent)
	}
	if bsdutils := byRef["pkg:deb/alpine/bsdutils@2.41-5?arch=amd64&distro=alpine-3.24.1&epoch=1&upstream=util-linux%402.41-5"]; bsdutils != nil && bsdutils.Version != "1:2.41-5" {
		t.Errorf("expected the component version to keep the epoch, got %q", bsdutils.Version)
	}
	if musl := byRef["pkg:apk/alpine/musl@1.2.6-r2?arch=x86_64&distro=alpine-3.24.1"]; musl != nil && (len(musl.Licenses) != 1 || musl.Licenses[0].License == nil || musl.Licenses[0].License.Id != "MIT") {
		t.Errorf("expected the apk license, got %+v", musl.Licenses)
	}
}

func TestImageComponentsWithoutOsRelease(t *testing.T) {
	t.Parallel()

	analysis := &image.Analysis{
		Reference:   "app",
		ApkPackages: []*ospkg.ApkPackage{{Name: "musl", Version: "1.2.6-r2", Arch: "x86_64"}},
	}
	components, err := ImageComponents(analysis, altshiftSbomTypes.ScopeRequired)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(components) != 1 || components[0] == nil {
		t.Fatalf("expected one component, got %v", refs(components))
	}
	if components[0].Purl != "pkg:apk/alpine/musl@1.2.6-r2?arch=x86_64" {
		t.Errorf("expected the apk package without a distro qualifier, got %v", refs(components))
	}
	if len(components[0].Properties) != 1 || components[0].Properties[0].Value != "app" {
		t.Errorf("expected the reference as image property, got %+v", components[0].Properties)
	}
	if components, err := ImageComponents(nil, altshiftSbomTypes.ScopeRequired); components != nil || err != nil {
		t.Errorf("expected nil for a nil analysis, got %v %v", components, err)
	}
}

func TestContainerComponentAndNest(t *testing.T) {
	t.Parallel()

	analysis := &image.Analysis{Reference: "golang:1.26-alpine", Id: "sha256:0fe4", RepoTags: []string{"docker.io/library/golang:1.26-alpine"}}
	container := ContainerComponent("golang:1.26-alpine", analysis, altshiftSbomTypes.ScopeExcluded)
	if container.Type != altshiftSbomTypes.ComponentTypeContainer || container.Name != "golang" || container.Version != "1.26-alpine" || container.Purl != "pkg:docker/golang@1.26-alpine" || container.BomRef != container.Purl || container.Scope != altshiftSbomTypes.ScopeExcluded {
		t.Errorf("unexpected container component: %+v", container)
	}
	if len(container.Properties) != 2 || container.Properties[0].Value != "docker.io/library/golang:1.26-alpine" || container.Properties[1].Name != altshiftSbomTypes.PropertyImageId || container.Properties[1].Value != "sha256:0fe4" {
		t.Errorf("unexpected container properties: %+v", container.Properties)
	}

	plain := ContainerComponent("docker.io/alpine:3.24", nil, altshiftSbomTypes.ScopeRequired)
	if plain.Purl != "pkg:docker/docker.io/alpine@3.24" || len(plain.Properties) != 0 {
		t.Errorf("unexpected plain container component: %+v", plain)
	}

	child := &altshiftSbomTypes.Component{Type: altshiftSbomTypes.ComponentTypeLibrary, Name: "musl", BomRef: "pkg:apk/alpine/musl@1", Components: []*altshiftSbomTypes.Component{{Name: "grandchild", BomRef: "gc"}}}
	Nest(container, []*altshiftSbomTypes.Component{child, nil})
	if len(container.Components) != 1 || container.Components[0].BomRef != "pkg:docker/golang@1.26-alpine/pkg:apk/alpine/musl@1" || container.Components[0].Components[0].BomRef != "pkg:docker/golang@1.26-alpine/gc" {
		t.Errorf("expected nested bom-refs to be prefixed, got %+v", container.Components)
	}
	if Nest(nil, nil) != nil {
		t.Errorf("expected nil for a nil container")
	}
}

func refs(components []*altshiftSbomTypes.Component) []string {
	var result []string
	for _, component := range components {
		result = append(result, component.BomRef)
	}
	return result
}
