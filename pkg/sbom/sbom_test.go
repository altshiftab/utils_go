package sbom

import (
	"encoding/json/v2"
	"errors"
	"slices"
	"testing"

	altshiftSbomTypes "github.com/altshiftab/utils_go/pkg/sbom/types"
)

func TestGoModulePurl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		version  string
		expected string
	}{
		{
			name:     "simple module",
			path:     "github.com/foo/bar",
			version:  "v1.2.3",
			expected: "pkg:golang/github.com/foo/bar@v1.2.3",
		},
		{
			name:     "no version",
			path:     "github.com/foo/bar",
			version:  "",
			expected: "pkg:golang/github.com/foo/bar",
		},
		{
			name:     "module with major version suffix",
			path:     "github.com/foo/bar/v2",
			version:  "v2.0.0",
			expected: "pkg:golang/github.com/foo/bar/v2@v2.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := goModulePurl(tt.path, tt.version)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestNpmPurl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pkg      string
		version  string
		expected string
	}{
		{
			name:     "simple package",
			pkg:      "express",
			version:  "4.18.2",
			expected: "pkg:npm/express@4.18.2",
		},
		{
			name:     "scoped package",
			pkg:      "@types/node",
			version:  "20.0.0",
			expected: "pkg:npm/%40types/node@20.0.0",
		},
		{
			name:     "no version",
			pkg:      "lodash",
			version:  "",
			expected: "pkg:npm/lodash",
		},
		{
			name:     "scoped no version",
			pkg:      "@angular/core",
			version:  "",
			expected: "pkg:npm/%40angular/core",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := npmPurl(tt.pkg, tt.version)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDockerPurl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		image    string
		version  string
		expected string
	}{
		{
			name:     "simple image",
			image:    "nginx",
			version:  "1.25",
			expected: "pkg:docker/nginx@1.25",
		},
		{
			name:     "namespaced image",
			image:    "library/nginx",
			version:  "latest",
			expected: "pkg:docker/library/nginx@latest",
		},
		{
			name:     "no version",
			image:    "alpine",
			version:  "",
			expected: "pkg:docker/alpine",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := dockerPurl(tt.image, tt.version)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestParseGoModules(t *testing.T) {
	t.Parallel()

	goListOutput := []byte(`{"Path":"github.com/altshiftab/utils_go","Main":true,"Dir":"/home/user/project","GoMod":"/home/user/project/go.mod"}
{"Path":"github.com/foo/bar","Version":"v1.0.0","Dir":"/home/user/go/pkg/mod/github.com/foo/bar@v1.0.0"}
{"Path":"github.com/baz/qux","Version":"v2.3.4","Dir":"/home/user/go/pkg/mod/github.com/baz/qux@v2.3.4"}
`)

	components, err := ParseGoModules(goListOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(components))
	}

	if components[0].Name != "github.com/foo/bar" {
		t.Errorf("expected name %q, got %q", "github.com/foo/bar", components[0].Name)
	}

	if components[0].Version != "v1.0.0" {
		t.Errorf("expected version %q, got %q", "v1.0.0", components[0].Version)
	}

	if components[0].Type != altshiftSbomTypes.ComponentTypeLibrary {
		t.Errorf("expected type %q, got %q", altshiftSbomTypes.ComponentTypeLibrary, components[0].Type)
	}

	if components[0].Purl != "pkg:golang/github.com/foo/bar@v1.0.0" {
		t.Errorf("expected purl %q, got %q", "pkg:golang/github.com/foo/bar@v1.0.0", components[0].Purl)
	}

	if components[1].Name != "github.com/baz/qux" {
		t.Errorf("expected name %q, got %q", "github.com/baz/qux", components[1].Name)
	}
}

func TestParseGoModulesEmpty(t *testing.T) {
	t.Parallel()

	components, err := ParseGoModules(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if components != nil {
		t.Errorf("expected nil, got %v", components)
	}
}

func TestParseGoModulesInvalidJson(t *testing.T) {
	t.Parallel()

	_, err := ParseGoModules([]byte(`{invalid json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseGoModulesSkipsMainModule(t *testing.T) {
	t.Parallel()

	goListOutput := []byte(`{"Path":"myproject","Main":true}
`)

	components, err := ParseGoModules(goListOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(components) != 0 {
		t.Errorf("expected 0 components, got %d", len(components))
	}
}

func TestParseNodePackageLock(t *testing.T) {
	t.Parallel()

	data := []byte(`{
  "name": "my-app",
  "version": "1.0.0",
  "packages": {
    "": {
      "name": "my-app",
      "version": "1.0.0"
    },
    "node_modules/express": {
      "version": "4.18.2",
      "license": "MIT"
    },
    "node_modules/lodash": {
      "version": "4.17.21",
      "license": "MIT"
    }
  }
}`)

	components, err := ParseNodePackageLock(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(components))
	}

	names := make([]string, len(components))
	for i, c := range components {
		names[i] = c.Name
	}

	if !slices.Contains(names, "express") {
		t.Error("expected express in components")
	}

	if !slices.Contains(names, "lodash") {
		t.Error("expected lodash in components")
	}

	for _, c := range components {
		if c.Name == "express" {
			if c.Version != "4.18.2" {
				t.Errorf("expected express version %q, got %q", "4.18.2", c.Version)
			}
			if c.Purl != "pkg:npm/express@4.18.2" {
				t.Errorf("expected purl %q, got %q", "pkg:npm/express@4.18.2", c.Purl)
			}
			if len(c.Licenses) != 1 || c.Licenses[0].License == nil || c.Licenses[0].License.Id != "MIT" {
				t.Errorf("expected MIT license, got %v", c.Licenses)
			}
		}
	}
}

func TestParseNodePackageLockWithScopedPackages(t *testing.T) {
	t.Parallel()

	data := []byte(`{
  "name": "my-app",
  "version": "1.0.0",
  "packages": {
    "": {
      "name": "my-app",
      "version": "1.0.0"
    },
    "node_modules/@types/node": {
      "version": "20.0.0",
      "license": "MIT"
    }
  }
}`)

	components, err := ParseNodePackageLock(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(components))
	}

	if components[0].Name != "@types/node" {
		t.Errorf("expected name %q, got %q", "@types/node", components[0].Name)
	}

	if components[0].Purl != "pkg:npm/%40types/node@20.0.0" {
		t.Errorf("expected purl %q, got %q", "pkg:npm/%40types/node@20.0.0", components[0].Purl)
	}
}

func TestParseNodePackageLockEmpty(t *testing.T) {
	t.Parallel()

	components, err := ParseNodePackageLock(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if components != nil {
		t.Errorf("expected nil, got %v", components)
	}
}

func TestParseNodePackageLockInvalidJson(t *testing.T) {
	t.Parallel()

	_, err := ParseNodePackageLock([]byte(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseNodePackageLockFallbackToDependencies(t *testing.T) {
	t.Parallel()

	data := []byte(`{
  "name": "my-app",
  "version": "1.0.0",
  "dependencies": {
    "express": {
      "version": "4.18.2",
      "requires": {}
    }
  }
}`)

	components, err := ParseNodePackageLock(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(components))
	}

	if components[0].Name != "express" {
		t.Errorf("expected name %q, got %q", "express", components[0].Name)
	}

	if components[0].Version != "4.18.2" {
		t.Errorf("expected version %q, got %q", "4.18.2", components[0].Version)
	}
}

func TestParseNodePackageLockNestedDependencies(t *testing.T) {
	t.Parallel()

	data := []byte(`{
  "name": "my-app",
  "version": "1.0.0",
  "dependencies": {
    "express": {
      "version": "4.18.2",
      "dependencies": {
        "body-parser": {
          "version": "1.20.1"
        }
      }
    }
  }
}`)

	components, err := ParseNodePackageLock(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(components))
	}

	names := make([]string, len(components))
	for i, c := range components {
		names[i] = c.Name
	}

	if !slices.Contains(names, "express") {
		t.Error("expected express in components")
	}

	if !slices.Contains(names, "body-parser") {
		t.Error("expected body-parser in components")
	}
}

func TestExtractNodeModuleName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "simple",
			path:     "node_modules/express",
			expected: "express",
		},
		{
			name:     "scoped",
			path:     "node_modules/@types/node",
			expected: "@types/node",
		},
		{
			name:     "nested",
			path:     "node_modules/express/node_modules/qs",
			expected: "qs",
		},
		{
			name:     "no node_modules",
			path:     "src/index.js",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := extractNodeModuleName(tt.path)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestParseGoSum(t *testing.T) {
	t.Parallel()

	goSumContent := []byte(`github.com/foo/bar v1.0.0 h1:abc123=
github.com/foo/bar v1.0.0/go.mod h1:def456=
github.com/baz/qux v2.3.4 h1:ghi789=
github.com/baz/qux v2.3.4/go.mod h1:jkl012=
github.com/transitive/dep v0.5.0 h1:mno345=
github.com/transitive/dep v0.5.0/go.mod h1:pqr678=
`)

	components, err := ParseGoSum(goSumContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(components) != 3 {
		t.Fatalf("expected 3 components, got %d", len(components))
	}

	names := make(map[string]string)
	for _, c := range components {
		names[c.Name] = c.Version
	}

	if names["github.com/foo/bar"] != "v1.0.0" {
		t.Errorf("expected foo/bar v1.0.0, got %s", names["github.com/foo/bar"])
	}

	if names["github.com/baz/qux"] != "v2.3.4" {
		t.Errorf("expected baz/qux v2.3.4, got %s", names["github.com/baz/qux"])
	}

	if names["github.com/transitive/dep"] != "v0.5.0" {
		t.Errorf("expected transitive/dep v0.5.0, got %s", names["github.com/transitive/dep"])
	}

	for _, c := range components {
		if c.Type != altshiftSbomTypes.ComponentTypeLibrary {
			t.Errorf("expected type %q, got %q", altshiftSbomTypes.ComponentTypeLibrary, c.Type)
		}
		if c.Purl == "" {
			t.Errorf("expected non-empty purl for %s", c.Name)
		}
	}
}

func TestParseGoSumEmpty(t *testing.T) {
	t.Parallel()

	components, err := ParseGoSum(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if components != nil {
		t.Errorf("expected nil, got %v", components)
	}
}

func TestParseGoSumSkipsGoModEntries(t *testing.T) {
	t.Parallel()

	// Only go.mod entries — should produce no components.
	goSumContent := []byte(`github.com/foo/bar v1.0.0/go.mod h1:abc123=
github.com/baz/qux v2.3.4/go.mod h1:def456=
`)

	components, err := ParseGoSum(goSumContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(components) != 0 {
		t.Errorf("expected 0 components, got %d", len(components))
	}
}

func TestParseGoSumDeduplicates(t *testing.T) {
	t.Parallel()

	goSumContent := []byte(`github.com/foo/bar v1.0.0 h1:abc123=
github.com/foo/bar v1.0.0 h1:abc123=
github.com/foo/bar v1.0.0/go.mod h1:def456=
`)

	components, err := ParseGoSum(goSumContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(components))
	}

	if components[0].Name != "github.com/foo/bar" || components[0].Version != "v1.0.0" {
		t.Errorf("unexpected component: %+v", components[0])
	}
}

func TestParseDockerfile(t *testing.T) {
	t.Parallel()

	data := []byte(`FROM golang:1.21 AS builder
RUN go build -o /app .

FROM alpine:3.18
COPY --from=builder /app /app
CMD ["/app"]
`)

	components, err := ParseDockerfile(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(components))
	}

	names := make([]string, len(components))
	for i, c := range components {
		names[i] = c.Name
	}

	if !slices.Contains(names, "golang") {
		t.Error("expected golang in components")
	}

	if !slices.Contains(names, "alpine") {
		t.Error("expected alpine in components")
	}

	for _, c := range components {
		if c.Type != altshiftSbomTypes.ComponentTypeContainer {
			t.Errorf("expected type %q, got %q", altshiftSbomTypes.ComponentTypeContainer, c.Type)
		}
		if c.Name == "golang" {
			if c.Version != "1.21" {
				t.Errorf("expected version %q, got %q", "1.21", c.Version)
			}
			if c.Purl != "pkg:docker/golang@1.21" {
				t.Errorf("expected purl %q, got %q", "pkg:docker/golang@1.21", c.Purl)
			}
		}
		if c.Name == "alpine" {
			if c.Version != "3.18" {
				t.Errorf("expected version %q, got %q", "3.18", c.Version)
			}
		}
	}
}

func TestParseDockerfileEmpty(t *testing.T) {
	t.Parallel()

	components, err := ParseDockerfile(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if components != nil {
		t.Errorf("expected nil, got %v", components)
	}
}

func TestParseDockerfileSkipsScratch(t *testing.T) {
	t.Parallel()

	data := []byte(`FROM scratch
COPY /app /app
`)

	components, err := ParseDockerfile(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(components) != 0 {
		t.Errorf("expected 0 components, got %d", len(components))
	}
}

func TestParseDockerfileWithPlatform(t *testing.T) {
	t.Parallel()

	data := []byte(`FROM --platform=linux/amd64 nginx:1.25
`)

	components, err := ParseDockerfile(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(components))
	}

	if components[0].Name != "nginx" {
		t.Errorf("expected name %q, got %q", "nginx", components[0].Name)
	}

	if components[0].Version != "1.25" {
		t.Errorf("expected version %q, got %q", "1.25", components[0].Version)
	}
}

func TestParseDockerfileWithDigest(t *testing.T) {
	t.Parallel()

	data := []byte(`FROM nginx@sha256:abc123
`)

	components, err := ParseDockerfile(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(components))
	}

	if components[0].Name != "nginx" {
		t.Errorf("expected name %q, got %q", "nginx", components[0].Name)
	}

	if components[0].Version != "sha256:abc123" {
		t.Errorf("expected version %q, got %q", "sha256:abc123", components[0].Version)
	}
}

func TestParseDockerfileDeduplicates(t *testing.T) {
	t.Parallel()

	data := []byte(`FROM golang:1.21 AS builder
FROM golang:1.21 AS tester
FROM alpine:3.18
`)

	components, err := ParseDockerfile(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(components) != 2 {
		t.Fatalf("expected 2 components (deduplicated), got %d", len(components))
	}
}

func TestParseDockerfileNamespaced(t *testing.T) {
	t.Parallel()

	data := []byte(`FROM docker.io/library/nginx:latest
`)

	components, err := ParseDockerfile(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(components))
	}

	if components[0].Name != "docker.io/library/nginx" {
		t.Errorf("expected name %q, got %q", "docker.io/library/nginx", components[0].Name)
	}

	if components[0].Version != "latest" {
		t.Errorf("expected version %q, got %q", "latest", components[0].Version)
	}
}

func TestGenerateBom(t *testing.T) {
	t.Parallel()

	components := []*altshiftSbomTypes.Component{
		{
			Type:    altshiftSbomTypes.ComponentTypeLibrary,
			Name:    "github.com/foo/bar",
			Version: "v1.0.0",
			Purl:    "pkg:golang/github.com/foo/bar@v1.0.0",
			BomRef:  "pkg:golang/github.com/foo/bar@v1.0.0",
		},
	}

	bom, err := GenerateBom(nil, components)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bom.BomFormat != altshiftSbomTypes.BomFormatCycloneDX {
		t.Errorf("expected bomFormat %q, got %q", altshiftSbomTypes.BomFormatCycloneDX, bom.BomFormat)
	}

	if bom.SpecVersion != "1.6" {
		t.Errorf("expected specVersion %q, got %q", "1.6", bom.SpecVersion)
	}

	if bom.Version != 1 {
		t.Errorf("expected version 1, got %d", bom.Version)
	}

	if bom.Metadata == nil {
		t.Fatal("expected metadata to be set")
	}

	if len(bom.Metadata.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(bom.Metadata.Tools))
	}

	if len(bom.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(bom.Components))
	}
}

func TestGenerateBomMergesAndSorts(t *testing.T) {
	t.Parallel()

	components := []*altshiftSbomTypes.Component{
		{Type: altshiftSbomTypes.ComponentTypeLibrary, Name: "zlib", Version: "1.0", Purl: "pkg:apk/alpine/zlib@1.0", BomRef: "pkg:apk/alpine/zlib@1.0"},
		{
			Type: altshiftSbomTypes.ComponentTypeLibrary, Name: "stdlib", Version: "1.26.6", Purl: "pkg:golang/stdlib@1.26.6", BomRef: "pkg:golang/stdlib@1.26.6",
			Properties: []*altshiftSbomTypes.Property{{Name: altshiftSbomTypes.PropertyPath, Value: "/usr/local/go/bin/go"}},
		},
		{
			Type: altshiftSbomTypes.ComponentTypeLibrary, Name: "stdlib", Version: "1.26.6", Purl: "pkg:golang/stdlib@1.26.6", BomRef: "pkg:golang/stdlib@1.26.6",
			Properties: []*altshiftSbomTypes.Property{{Name: altshiftSbomTypes.PropertyPath, Value: "/usr/local/go/bin/gofmt"}, {Name: altshiftSbomTypes.PropertyPath, Value: "/usr/local/go/bin/go"}},
		},
		{Type: altshiftSbomTypes.ComponentTypeLibrary, Name: "no-ref", Version: "1"},
	}

	bom, err := GenerateBom(nil, components)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(bom.Components) != 3 {
		t.Fatalf("expected 3 components after merging, got %d", len(bom.Components))
	}
	if bom.Components[0].BomRef != "pkg:apk/alpine/zlib@1.0" || bom.Components[1].BomRef != "pkg:golang/stdlib@1.26.6" || bom.Components[2].Name != "no-ref" {
		t.Errorf("expected bom-ref order with unreferenced last, got %v %v %v", bom.Components[0].BomRef, bom.Components[1].BomRef, bom.Components[2].Name)
	}
	if paths := bom.Components[1].Properties; len(paths) != 2 || paths[0].Value != "/usr/local/go/bin/go" || paths[1].Value != "/usr/local/go/bin/gofmt" {
		t.Errorf("expected the two paths merged, got %+v", paths)
	}
}

func TestGenerateBomRejectsConflictingBomRefs(t *testing.T) {
	t.Parallel()

	components := []*altshiftSbomTypes.Component{
		{Type: altshiftSbomTypes.ComponentTypeLibrary, Name: "a", Version: "1", BomRef: "same"},
		{Type: altshiftSbomTypes.ComponentTypeLibrary, Name: "b", Version: "2", BomRef: "same"},
	}

	if _, err := GenerateBom(nil, components); !errors.Is(err, ErrBomRefConflict) {
		t.Fatalf("expected a bom-ref conflict, got %v", err)
	}
}

func TestGenerateBomJson(t *testing.T) {
	t.Parallel()

	components := []*altshiftSbomTypes.Component{
		{
			Type:    altshiftSbomTypes.ComponentTypeLibrary,
			Name:    "express",
			Version: "4.18.2",
			Purl:    "pkg:npm/express@4.18.2",
			BomRef:  "pkg:npm/express@4.18.2",
		},
	}

	data, err := GenerateBomJson(nil, components)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var bom altshiftSbomTypes.Bom
	if err := json.Unmarshal(data, &bom); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if bom.BomFormat != altshiftSbomTypes.BomFormatCycloneDX {
		t.Errorf("expected bomFormat %q, got %q", altshiftSbomTypes.BomFormatCycloneDX, bom.BomFormat)
	}

	if len(bom.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(bom.Components))
	}

	if bom.Components[0].Purl != "pkg:npm/express@4.18.2" {
		t.Errorf("expected purl %q, got %q", "pkg:npm/express@4.18.2", bom.Components[0].Purl)
	}
}

func TestCycloneDxJsonStructure(t *testing.T) {
	t.Parallel()

	components := []*altshiftSbomTypes.Component{
		{
			Type:    altshiftSbomTypes.ComponentTypeLibrary,
			Name:    "github.com/foo/bar",
			Version: "v1.0.0",
			Purl:    "pkg:golang/github.com/foo/bar@v1.0.0",
			BomRef:  "pkg:golang/github.com/foo/bar@v1.0.0",
		},
	}

	data, err := GenerateBomJson(nil, components)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if raw["bomFormat"] != "CycloneDX" {
		t.Errorf("expected bomFormat %q, got %v", "CycloneDX", raw["bomFormat"])
	}

	if raw["specVersion"] != "1.6" {
		t.Errorf("expected specVersion %q, got %v", "1.6", raw["specVersion"])
	}

	if raw["version"] != float64(1) {
		t.Errorf("expected version 1, got %v", raw["version"])
	}

	componentsRaw, ok := raw["components"].([]any)
	if !ok {
		t.Fatal("expected components to be an array")
	}

	if len(componentsRaw) != 1 {
		t.Fatalf("expected 1 component, got %d", len(componentsRaw))
	}

	comp, ok := componentsRaw[0].(map[string]any)
	if !ok {
		t.Fatal("expected component to be an object")
	}

	if comp["type"] != "library" {
		t.Errorf("expected type %q, got %v", "library", comp["type"])
	}

	if comp["purl"] != "pkg:golang/github.com/foo/bar@v1.0.0" {
		t.Errorf("expected purl %q, got %v", "pkg:golang/github.com/foo/bar@v1.0.0", comp["purl"])
	}

	if comp["bom-ref"] != "pkg:golang/github.com/foo/bar@v1.0.0" {
		t.Errorf("expected bom-ref %q, got %v", "pkg:golang/github.com/foo/bar@v1.0.0", comp["bom-ref"])
	}
}

func TestNodePackageScope(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		detail   *nodePackageDetail
		expected altshiftSbomTypes.Scope
	}{
		{name: "runtime dependency", detail: &nodePackageDetail{}, expected: altshiftSbomTypes.ScopeRequired},
		{name: "dev dependency", detail: &nodePackageDetail{Dev: true}, expected: altshiftSbomTypes.ScopeExcluded},
		{name: "dev optional dependency", detail: &nodePackageDetail{DevOptional: true}, expected: altshiftSbomTypes.ScopeExcluded},
		{name: "optional dependency", detail: &nodePackageDetail{Optional: true}, expected: altshiftSbomTypes.ScopeOptional},
		{name: "dev wins over optional", detail: &nodePackageDetail{Dev: true, Optional: true}, expected: altshiftSbomTypes.ScopeExcluded},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := nodePackageScope(testCase.detail); got != testCase.expected {
				t.Errorf("expected %q, got %q", testCase.expected, got)
			}
		})
	}
}

func TestParseNodePackageLockScopes(t *testing.T) {
	t.Parallel()

	data := []byte(`{
		"name": "app", "version": "1.0.0", "lockfileVersion": 3,
		"packages": {
			"": {"name": "app", "version": "1.0.0"},
			"node_modules/lit": {"version": "3.3.0"},
			"node_modules/typescript": {"version": "5.9.2", "dev": true},
			"node_modules/fsevents": {"version": "2.3.3", "optional": true},
			"node_modules/postcss": {"version": "8.5.16", "devOptional": true}
		}
	}`)

	components, err := ParseNodePackageLock(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	scopes := make(map[string]altshiftSbomTypes.Scope)
	for _, component := range components {
		scopes[component.Name] = component.Scope
	}
	expected := map[string]altshiftSbomTypes.Scope{
		"lit":        altshiftSbomTypes.ScopeRequired,
		"typescript": altshiftSbomTypes.ScopeExcluded,
		"fsevents":   altshiftSbomTypes.ScopeOptional,
		"postcss":    altshiftSbomTypes.ScopeExcluded,
	}
	if len(scopes) != len(expected) {
		t.Fatalf("expected %d components, got %d: %v", len(expected), len(scopes), scopes)
	}
	for name, scope := range expected {
		if scopes[name] != scope {
			t.Errorf("%s: expected scope %q, got %q", name, scope, scopes[name])
		}
	}
}

func TestGenerateBomJsonNestedStructure(t *testing.T) {
	t.Parallel()

	subject := &altshiftSbomTypes.Component{
		Type:   altshiftSbomTypes.ComponentTypeContainer,
		Name:   "app",
		Purl:   "pkg:docker/app",
		BomRef: "pkg:docker/app",
		Properties: []*altshiftSbomTypes.Property{
			{Name: altshiftSbomTypes.PropertyImage, Value: "localhost/app:latest"},
		},
	}
	components := []*altshiftSbomTypes.Component{
		{
			Type:    altshiftSbomTypes.ComponentTypeContainer,
			Name:    "golang",
			Version: "1.26-alpine",
			Scope:   altshiftSbomTypes.ScopeExcluded,
			Purl:    "pkg:docker/golang@1.26-alpine",
			BomRef:  "pkg:docker/golang@1.26-alpine",
			Components: []*altshiftSbomTypes.Component{
				{
					Type:    altshiftSbomTypes.ComponentTypeLibrary,
					Name:    "musl",
					Version: "1.2.6-r2",
					Scope:   altshiftSbomTypes.ScopeExcluded,
					Purl:    "pkg:apk/alpine/musl@1.2.6-r2?arch=x86_64&distro=alpine-3.24.1",
					BomRef:  "pkg:docker/golang@1.26-alpine/pkg:apk/alpine/musl@1.2.6-r2?arch=x86_64&distro=alpine-3.24.1",
					Properties: []*altshiftSbomTypes.Property{
						{Name: altshiftSbomTypes.PropertyPath, Value: "/lib/apk/db/installed"},
					},
				},
			},
		},
	}

	data, err := GenerateBomJson(subject, components)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	metadata, _ := raw["metadata"].(map[string]any)
	metadataComponent, _ := metadata["component"].(map[string]any)
	if metadataComponent["bom-ref"] != "pkg:docker/app" {
		t.Errorf("expected the subject as metadata.component, got %v", metadata["component"])
	}
	properties, _ := metadataComponent["properties"].([]any)
	if len(properties) != 1 {
		t.Errorf("expected the subject's properties to be written, got %v", metadataComponent["properties"])
	}

	componentsRaw, _ := raw["components"].([]any)
	if len(componentsRaw) != 1 {
		t.Fatalf("expected 1 top-level component, got %v", raw["components"])
	}
	container, _ := componentsRaw[0].(map[string]any)
	if container["scope"] != "excluded" {
		t.Errorf("expected scope excluded, got %v", container["scope"])
	}
	nested, _ := container["components"].([]any)
	if len(nested) != 1 {
		t.Fatalf("expected 1 nested component, got %v", container["components"])
	}
	musl, _ := nested[0].(map[string]any)
	if musl["scope"] != "excluded" || musl["purl"] != "pkg:apk/alpine/musl@1.2.6-r2?arch=x86_64&distro=alpine-3.24.1" {
		t.Errorf("unexpected nested component: %v", musl)
	}
	if _, present := musl["hashes"]; present {
		t.Errorf("expected empty optional fields to be omitted, got %v", musl)
	}
}
