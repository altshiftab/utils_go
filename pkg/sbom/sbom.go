package sbom

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftSbomTypes "github.com/altshiftab/utils_go/pkg/sbom/types"
)

const (
	specVersion = "1.6"
	bomVersion  = 1
	toolName    = "motmedel-sbom-generator"
	toolVersion = "0.1.0"
)

type goModuleInfo struct {
	Path    string `json:"Path"`
	Version string `json:"Version"`
	Dir     string `json:"Dir"`
	GoMod   string `json:"GoMod"`
	Main    bool   `json:"Main"`
}

type nodePackageLock struct {
	Name         string                        `json:"name"`
	Version      string                        `json:"version"`
	Packages     map[string]*nodePackageDetail `json:"packages"`
	Dependencies map[string]*nodeDependency    `json:"dependencies"`
}

type nodePackageDetail struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	License string `json:"license"`
}

type nodeDependency struct {
	Version      string                     `json:"version"`
	Requires     map[string]string          `json:"requires"`
	Dependencies map[string]*nodeDependency `json:"dependencies"`
}

var dockerfileFromRegexp = regexp.MustCompile(`(?im)^\s*FROM\s+(?:--platform=\S+\s+)?(\S+?)(?:\s+AS\s+\S+)?\s*$`)

func goModulePurl(path string, version string) string {
	escapedPath := url.PathEscape(path)
	escapedPath = strings.ReplaceAll(escapedPath, "%2F", "/")

	if version == "" {
		return fmt.Sprintf("pkg:golang/%s", escapedPath)
	}

	return fmt.Sprintf("pkg:golang/%s@%s", escapedPath, version)
}

func npmPurl(name string, version string) string {
	if strings.HasPrefix(name, "@") {
		parts := strings.SplitN(name, "/", 2)
		if len(parts) == 2 {
			namespace := url.PathEscape(parts[0][1:])
			pkg := url.PathEscape(parts[1])

			if version == "" {
				return fmt.Sprintf("pkg:npm/%%40%s/%s", namespace, pkg)
			}

			return fmt.Sprintf("pkg:npm/%%40%s/%s@%s", namespace, pkg, version)
		}
	}

	escapedName := url.PathEscape(name)

	if version == "" {
		return fmt.Sprintf("pkg:npm/%s", escapedName)
	}

	return fmt.Sprintf("pkg:npm/%s@%s", escapedName, version)
}

func dockerPurl(name string, version string) string {
	parts := strings.SplitN(name, "/", 2)

	var namespace, pkg string
	if len(parts) == 2 {
		namespace = url.PathEscape(parts[0])
		pkg = url.PathEscape(parts[1])
	} else {
		pkg = url.PathEscape(name)
	}

	var purlBase string
	if namespace != "" {
		purlBase = fmt.Sprintf("pkg:docker/%s/%s", namespace, pkg)
	} else {
		purlBase = fmt.Sprintf("pkg:docker/%s", pkg)
	}

	if version == "" {
		return purlBase
	}

	return fmt.Sprintf("%s@%s", purlBase, version)
}

func ParseGoModules(goListOutput []byte) ([]altshiftSbomTypes.Component, error) {
	if len(goListOutput) == 0 {
		return nil, nil
	}

	var components []altshiftSbomTypes.Component

	decoder := jsontext.NewDecoder(bytes.NewReader(goListOutput))
	for {
		var module goModuleInfo
		if err := json.UnmarshalDecode(decoder, &module); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("json unmarshal decode: %w", err),
				decoder, module,
			)
		}

		if module.Main {
			continue
		}

		if module.Path == "" {
			continue
		}

		version := module.Version
		purl := goModulePurl(module.Path, version)

		components = append(components, altshiftSbomTypes.Component{
			Type:    altshiftSbomTypes.ComponentTypeLibrary,
			Name:    module.Path,
			Version: version,
			Purl:    purl,
			BomRef:  purl,
		})
	}

	return components, nil
}

func ParseNodePackageLock(data []byte) ([]altshiftSbomTypes.Component, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var packageLock nodePackageLock
	if err := json.Unmarshal(data, &packageLock); err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("json unmarshal: %w", err), data)
	}

	var components []altshiftSbomTypes.Component
	seen := make(map[string]bool)

	for path, detail := range packageLock.Packages {
		if detail == nil {
			continue
		}

		if path == "" {
			continue
		}

		name := detail.Name
		if name == "" {
			name = extractNodeModuleName(path)
		}

		if name == "" {
			continue
		}

		version := detail.Version
		key := fmt.Sprintf("%s@%s", name, version)

		if seen[key] {
			continue
		}
		seen[key] = true

		purl := npmPurl(name, version)

		component := altshiftSbomTypes.Component{
			Type:    altshiftSbomTypes.ComponentTypeLibrary,
			Name:    name,
			Version: version,
			Purl:    purl,
			BomRef:  purl,
		}

		if detail.License != "" {
			component.Licenses = []altshiftSbomTypes.LicenseChoice{
				{License: &altshiftSbomTypes.License{Id: detail.License}},
			}
		}

		components = append(components, component)
	}

	if len(components) == 0 {
		for name, dep := range packageLock.Dependencies {
			if dep == nil {
				continue
			}

			collectNodeDependencies(name, dep, seen, &components)
		}
	}

	return components, nil
}

func collectNodeDependencies(name string, dep *nodeDependency, seen map[string]bool, components *[]altshiftSbomTypes.Component) {
	if dep == nil {
		return
	}

	version := dep.Version
	key := fmt.Sprintf("%s@%s", name, version)

	if seen[key] {
		return
	}
	seen[key] = true

	purl := npmPurl(name, version)

	*components = append(*components, altshiftSbomTypes.Component{
		Type:    altshiftSbomTypes.ComponentTypeLibrary,
		Name:    name,
		Version: version,
		Purl:    purl,
		BomRef:  purl,
	})

	for nestedName, nestedDep := range dep.Dependencies {
		collectNodeDependencies(nestedName, nestedDep, seen, components)
	}
}

func extractNodeModuleName(path string) string {
	const nodeModulesPrefix = "node_modules/"

	idx := strings.LastIndex(path, nodeModulesPrefix)
	if idx == -1 {
		return ""
	}

	return path[idx+len(nodeModulesPrefix):]
}

func ParseGoSum(data []byte) ([]altshiftSbomTypes.Component, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var components []altshiftSbomTypes.Component
	seen := make(map[string]bool)

	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		name := parts[0]
		version := parts[1]

		// go.sum has two entries per module: one for the module source and one
		// for the go.mod file (with a "/go.mod" suffix). Skip the go.mod entries
		// to avoid duplicates.
		if strings.HasSuffix(version, "/go.mod") {
			continue
		}

		key := fmt.Sprintf("%s@%s", name, version)
		if seen[key] {
			continue
		}
		seen[key] = true

		purl := goModulePurl(name, version)

		components = append(components, altshiftSbomTypes.Component{
			Type:    altshiftSbomTypes.ComponentTypeLibrary,
			Name:    name,
			Version: version,
			Purl:    purl,
			BomRef:  purl,
		})
	}

	return components, nil
}

func ParseDockerfile(data []byte) ([]altshiftSbomTypes.Component, error) {
	if len(data) == 0 {
		return nil, nil
	}

	matches := dockerfileFromRegexp.FindAllSubmatch(data, -1)
	if len(matches) == 0 {
		return nil, nil
	}

	var components []altshiftSbomTypes.Component
	seen := make(map[string]bool)

	for _, match := range matches {
		image := string(match[1])

		if strings.EqualFold(image, "scratch") {
			continue
		}

		if seen[image] {
			continue
		}
		seen[image] = true

		name := image
		var version string

		if atIdx := strings.LastIndex(image, "@"); atIdx != -1 {
			name = image[:atIdx]
			version = image[atIdx+1:]
		} else if colonIdx := strings.LastIndex(image, ":"); colonIdx != -1 {
			name = image[:colonIdx]
			version = image[colonIdx+1:]
		}

		purl := dockerPurl(name, version)

		components = append(components, altshiftSbomTypes.Component{
			Type:    altshiftSbomTypes.ComponentTypeContainer,
			Name:    name,
			Version: version,
			Purl:    purl,
			BomRef:  purl,
		})
	}

	return components, nil
}

func deduplicateComponents(components []altshiftSbomTypes.Component) []altshiftSbomTypes.Component {
	if len(components) == 0 {
		return components
	}

	seen := make(map[string]bool)
	result := make([]altshiftSbomTypes.Component, 0, len(components))

	for _, c := range components {
		key := fmt.Sprintf("%s|%s|%s", c.Type, c.Name, c.Version)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, c)
	}

	return result
}

func GenerateBom(components []altshiftSbomTypes.Component) *altshiftSbomTypes.Bom {
	return &altshiftSbomTypes.Bom{
		BomFormat:   altshiftSbomTypes.BomFormatCycloneDX,
		SpecVersion: specVersion,
		Version:     bomVersion,
		Metadata: &altshiftSbomTypes.Metadata{
			Tools: []altshiftSbomTypes.Tool{
				{Name: toolName, Version: toolVersion},
			},
		},
		Components: deduplicateComponents(components),
	}
}

func GenerateBomJson(components []altshiftSbomTypes.Component) ([]byte, error) {
	bom := GenerateBom(components)

	data, err := json.Marshal(bom, jsontext.WithIndent("  "))
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("json marshal indent: %w", err), bom)
	}

	return data, nil
}
