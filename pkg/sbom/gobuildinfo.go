package sbom

import (
	"debug/buildinfo"
	"runtime/debug"
	"strings"

	altshiftSbomTypes "github.com/altshiftab/utils_go/pkg/sbom/types"
)

const goStdlibName = "stdlib"

// BuildInfoComponents lists what a Go executable was built from, as its embedded build info records it: the main
// module (an application), the standard library at the Go version used, and each dependency module at the version
// linked (replacements taken into account). It is what a go.sum can only over-approximate. filePath, when given, is
// recorded on every component as where the executable was found.
func BuildInfoComponents(info *buildinfo.BuildInfo, filePath string, scope altshiftSbomTypes.Scope) []*altshiftSbomTypes.Component {
	if info == nil {
		return nil
	}

	var properties []*altshiftSbomTypes.Property
	if filePath != "" {
		properties = []*altshiftSbomTypes.Property{{Name: altshiftSbomTypes.PropertyPath, Value: filePath}}
	}
	// Each component gets its own property slice, so merging paths later cannot alias.
	copyProperties := func() []*altshiftSbomTypes.Property {
		if properties == nil {
			return nil
		}
		return []*altshiftSbomTypes.Property{{Name: properties[0].Name, Value: properties[0].Value}}
	}

	var components []*altshiftSbomTypes.Component

	if mainPath := strings.TrimSpace(info.Main.Path); mainPath != "" {
		version := goModuleVersion(info.Main.Version)
		purl := goModulePurl(mainPath, version)
		components = append(components, &altshiftSbomTypes.Component{
			Type:       altshiftSbomTypes.ComponentTypeApplication,
			Name:       mainPath,
			Version:    version,
			Scope:      scope,
			Purl:       purl,
			BomRef:     purl,
			Properties: copyProperties(),
		})
	}

	if goVersion := goStdlibVersion(info.GoVersion); goVersion != "" || info.GoVersion != "" {
		purl := goModulePurl(goStdlibName, goVersion)
		components = append(components, &altshiftSbomTypes.Component{
			Type:       altshiftSbomTypes.ComponentTypeLibrary,
			Name:       goStdlibName,
			Version:    goVersion,
			Scope:      scope,
			Purl:       purl,
			BomRef:     purl,
			Properties: copyProperties(),
		})
	}

	for _, dep := range info.Deps {
		if dep == nil {
			continue
		}
		module := dep
		if dep.Replace != nil {
			module = dep.Replace
		}
		modulePath := strings.TrimSpace(module.Path)
		// A replacement by a local directory is not a module anyone can look up.
		if modulePath == "" || strings.HasPrefix(modulePath, ".") || strings.HasPrefix(modulePath, "/") {
			continue
		}
		version := goModuleVersion(module.Version)
		purl := goModulePurl(modulePath, version)
		components = append(components, &altshiftSbomTypes.Component{
			Type:       altshiftSbomTypes.ComponentTypeLibrary,
			Name:       modulePath,
			Version:    version,
			Scope:      scope,
			Purl:       purl,
			BomRef:     purl,
			Properties: copyProperties(),
		})
	}

	return components
}

// goModuleVersion is the version as build info records it, except that the placeholder for an unversioned main module
// is dropped.
func goModuleVersion(version string) string {
	if version == "(devel)" {
		return ""
	}
	return version
}

// goStdlibVersion turns build info's Go version ("go1.26.6", "go1.26.6 X:nodwarf5", "devel go1.27-abc") into the
// standard library's version ("1.26.6"); a development toolchain has none.
func goStdlibVersion(goVersion string) string {
	goVersion, _, _ = strings.Cut(strings.TrimSpace(goVersion), " ")
	if goVersion == "" || strings.HasPrefix(goVersion, "devel") {
		return ""
	}
	return strings.TrimPrefix(goVersion, "go")
}

// A compile-time check that buildinfo.BuildInfo is debug.BuildInfo, which the tests build by hand.
var _ = func(info *debug.BuildInfo) *buildinfo.BuildInfo { return info }
