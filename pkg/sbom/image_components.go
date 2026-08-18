package sbom

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/sbom/image"
	"github.com/altshiftab/utils_go/pkg/sbom/ospkg"
	altshiftSbomTypes "github.com/altshiftab/utils_go/pkg/sbom/types"
)

// ErrBomRefConflict reports two different components claiming the same bom-ref.
var ErrBomRefConflict = errors.New("bom-ref conflict")

// ContainerComponent describes a container image. The analysis, when given, contributes the image's identity (its
// repo tag and ID); the reference is the image as named by the caller or the Dockerfile.
func ContainerComponent(reference string, analysis *image.Analysis, scope altshiftSbomTypes.Scope) *altshiftSbomTypes.Component {
	name, version := splitImageReference(reference)
	purl := dockerPurl(name, version)

	component := &altshiftSbomTypes.Component{
		Type:    altshiftSbomTypes.ComponentTypeContainer,
		Name:    name,
		Version: version,
		Scope:   scope,
		Purl:    purl,
		BomRef:  purl,
	}

	if analysis != nil {
		imageName := reference
		if len(analysis.RepoTags) != 0 {
			imageName = analysis.RepoTags[0]
		}
		component.Properties = append(component.Properties, &altshiftSbomTypes.Property{Name: altshiftSbomTypes.PropertyImage, Value: imageName})
		if analysis.Id != "" {
			component.Properties = append(component.Properties, &altshiftSbomTypes.Property{Name: altshiftSbomTypes.PropertyImageId, Value: analysis.Id})
		}
	}

	return component
}

// ImageComponents lists what an image analysis found as components: the operating system, its packages, the Go
// executables' modules and the shipped node packages, each carrying the image and the path it was read from.
// Components found several times in the image (the Go standard library in every Go binary) are one component with
// several paths.
func ImageComponents(analysis *image.Analysis, scope altshiftSbomTypes.Scope) []*altshiftSbomTypes.Component {
	if analysis == nil {
		return nil
	}

	imageName := analysis.Reference
	if len(analysis.RepoTags) != 0 {
		imageName = analysis.RepoTags[0]
	}
	imageProperty := func() *altshiftSbomTypes.Property {
		if imageName == "" {
			return nil
		}
		return &altshiftSbomTypes.Property{Name: altshiftSbomTypes.PropertyImage, Value: imageName}
	}
	pathProperty := func(filePath string) *altshiftSbomTypes.Property {
		if filePath == "" {
			return nil
		}
		return &altshiftSbomTypes.Property{Name: altshiftSbomTypes.PropertyPath, Value: "/" + strings.TrimPrefix(filePath, "/")}
	}
	layerProperty := func(layer string) *altshiftSbomTypes.Property {
		if layer == "" {
			return nil
		}
		return &altshiftSbomTypes.Property{Name: altshiftSbomTypes.PropertyLayer, Value: layer}
	}
	withProperties := func(component *altshiftSbomTypes.Component, filePath, layer string) *altshiftSbomTypes.Component {
		var properties []*altshiftSbomTypes.Property
		if property := imageProperty(); property != nil {
			properties = append(properties, property)
		}
		if property := pathProperty(filePath); property != nil {
			properties = append(properties, property)
		}
		if property := layerProperty(layer); property != nil {
			properties = append(properties, property)
		}
		component.Properties = append(properties, component.Properties...)
		return component
	}

	var components []*altshiftSbomTypes.Component

	var osId, distro string
	if osRelease := analysis.OsRelease; osRelease != nil && osRelease.Id != "" {
		osId = osRelease.Id
		distro = osRelease.Id
		if osRelease.VersionId != "" {
			distro += "-" + osRelease.VersionId
		}
		bomRef := "os:" + osRelease.Id
		if osRelease.VersionId != "" {
			bomRef += "@" + osRelease.VersionId
		}
		components = append(components, withProperties(&altshiftSbomTypes.Component{
			Type:    altshiftSbomTypes.ComponentTypeOperatingSystem,
			Name:    osRelease.Id,
			Version: osRelease.VersionId,
			Scope:   scope,
			BomRef:  bomRef,
		}, analysis.OsReleasePath, analysis.OsReleaseLayer))
	}

	for _, pkg := range analysis.ApkPackages {
		if pkg == nil || pkg.Name == "" {
			continue
		}
		components = append(components, withProperties(apkComponent(pkg, osId, distro, scope), analysis.ApkDatabasePath, analysis.ApkDatabaseLayer))
	}

	for _, status := range analysis.DpkgStatuses {
		if status == nil {
			continue
		}
		for _, pkg := range status.Packages {
			if pkg == nil || pkg.Name == "" {
				continue
			}
			components = append(components, withProperties(dpkgComponent(pkg, osId, distro, scope), status.Path, status.Layer))
		}
	}

	for _, binary := range analysis.GoBinaries {
		if binary == nil {
			continue
		}
		for _, component := range BuildInfoComponents(binary.Info, "", scope) {
			components = append(components, withProperties(component, binary.Path, binary.Layer))
		}
	}

	for _, pkg := range analysis.NodePackages {
		if pkg == nil || pkg.Name == "" || pkg.Version == "" {
			continue
		}
		purl := npmPurl(pkg.Name, pkg.Version)
		component := &altshiftSbomTypes.Component{
			Type:    altshiftSbomTypes.ComponentTypeLibrary,
			Name:    pkg.Name,
			Version: pkg.Version,
			Scope:   scope,
			Purl:    purl,
			BomRef:  purl,
		}
		if pkg.License != "" {
			component.Licenses = []*altshiftSbomTypes.LicenseChoice{{License: &altshiftSbomTypes.License{Id: pkg.License}}}
		}
		components = append(components, withProperties(component, pkg.Path, pkg.Layer))
	}

	merged, err := mergeComponents(components)
	if err != nil {
		// Components of one image only collide on equal bom-refs with equal content, which merge; anything else is
		// a bug in the mapping above and would surface in tests.
		return components
	}
	return merged
}

// apkComponent maps an Alpine package to "pkg:apk/<os>/<name>@<version>?arch=..&distro=..[&upstream=<origin>]".
func apkComponent(pkg *ospkg.ApkPackage, osId, distro string, scope altshiftSbomTypes.Scope) *altshiftSbomTypes.Component {
	qualifiers := map[string]string{"arch": pkg.Arch, "distro": distro}
	if pkg.Origin != "" && pkg.Origin != pkg.Name {
		qualifiers["upstream"] = pkg.Origin
	}
	purl := osPackagePurl("apk", cmp.Or(osId, "alpine"), pkg.Name, pkg.Version, qualifiers)

	component := &altshiftSbomTypes.Component{
		Type:    altshiftSbomTypes.ComponentTypeLibrary,
		Name:    pkg.Name,
		Version: pkg.Version,
		Scope:   scope,
		Purl:    purl,
		BomRef:  purl,
	}
	if pkg.License != "" {
		component.Licenses = []*altshiftSbomTypes.LicenseChoice{{License: &altshiftSbomTypes.License{Id: pkg.License}}}
	}
	return component
}

// dpkgComponent maps a Debian package to "pkg:deb/<os>/<name>@<version>?arch=..&distro=..[&epoch=..][&upstream=..]".
// The purl version has the epoch split off into a qualifier, as Trivy writes it; the component keeps the full version.
func dpkgComponent(pkg *ospkg.DpkgPackage, osId, distro string, scope altshiftSbomTypes.Scope) *altshiftSbomTypes.Component {
	qualifiers := map[string]string{"arch": pkg.Architecture, "distro": distro}
	version := pkg.Version
	if epoch, rest, ok := strings.Cut(pkg.Version, ":"); ok && epoch != "" && strings.Trim(epoch, "0123456789") == "" {
		if epoch != "0" {
			qualifiers["epoch"] = epoch
		}
		version = rest
	}
	if pkg.SourceName != "" && (pkg.SourceName != pkg.Name || pkg.SourceVersion != "") {
		upstream := pkg.SourceName
		if pkg.SourceVersion != "" {
			upstream += "@" + pkg.SourceVersion
		}
		qualifiers["upstream"] = upstream
	}
	purl := osPackagePurl("deb", cmp.Or(osId, "debian"), pkg.Name, version, qualifiers)

	return &altshiftSbomTypes.Component{
		Type:    altshiftSbomTypes.ComponentTypeLibrary,
		Name:    pkg.Name,
		Version: pkg.Version,
		Scope:   scope,
		Purl:    purl,
		BomRef:  purl,
	}
}

// osPackagePurl builds "pkg:<type>/<namespace>/<name>@<version>?<qualifiers>", qualifiers sorted by key and empty ones
// left out.
func osPackagePurl(purlType, namespace, name, version string, qualifiers map[string]string) string {
	purl := "pkg:" + purlType + "/" + escapePurlSegment(namespace) + "/" + escapePurlSegment(name)
	if version != "" {
		purl += "@" + escapePurlSegment(version)
	}

	keys := make([]string, 0, len(qualifiers))
	for key, value := range qualifiers {
		if value != "" {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	if len(keys) != 0 {
		pairs := make([]string, 0, len(keys))
		for _, key := range keys {
			pairs = append(pairs, key+"="+escapePurlSegment(qualifiers[key]))
		}
		purl += "?" + strings.Join(pairs, "&")
	}

	return purl
}

// Nest places components inside a container component, prefixing their bom-refs with the container's so that the
// same package in two images stays two components. It returns the container.
func Nest(container *altshiftSbomTypes.Component, children []*altshiftSbomTypes.Component) *altshiftSbomTypes.Component {
	if container == nil {
		return nil
	}
	for _, child := range children {
		if child == nil {
			continue
		}
		prefixBomRefs(child, container.BomRef)
		container.Components = append(container.Components, child)
	}
	return container
}

func prefixBomRefs(component *altshiftSbomTypes.Component, prefix string) {
	if prefix != "" && component.BomRef != "" {
		component.BomRef = prefix + "/" + component.BomRef
	}
	for _, child := range component.Components {
		if child != nil {
			prefixBomRefs(child, prefix)
		}
	}
}

// mergeComponents merges components that share a bom-ref, which must then agree on what they describe (their
// properties — the paths a package was found at — and nested components are combined), and orders the result by
// bom-ref, so that the output does not depend on the order things were found in. Components without a bom-ref are
// kept as they are, after the others.
func mergeComponents(components []*altshiftSbomTypes.Component) ([]*altshiftSbomTypes.Component, error) {
	byBomRef := make(map[string]*altshiftSbomTypes.Component)
	var merged, unreferenced []*altshiftSbomTypes.Component

	for _, component := range components {
		if component == nil {
			continue
		}
		if component.BomRef == "" {
			unreferenced = append(unreferenced, component)
			continue
		}
		existing, ok := byBomRef[component.BomRef]
		if !ok {
			byBomRef[component.BomRef] = component
			merged = append(merged, component)
			continue
		}
		if existing.Type != component.Type || existing.Name != component.Name || existing.Version != component.Version || existing.Purl != component.Purl || existing.Scope != component.Scope {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %s", ErrBomRefConflict, component.BomRef), existing, component)
		}
		existing.Properties = mergeProperties(existing.Properties, component.Properties)
		existing.Components = append(existing.Components, component.Components...)
	}

	for _, component := range merged {
		if len(component.Components) != 0 {
			children, err := mergeComponents(component.Components)
			if err != nil {
				return nil, err
			}
			component.Components = children
		}
	}

	slices.SortStableFunc(merged, func(a, b *altshiftSbomTypes.Component) int {
		return cmp.Compare(a.BomRef, b.BomRef)
	})
	return append(merged, unreferenced...), nil
}

func mergeProperties(a, b []*altshiftSbomTypes.Property) []*altshiftSbomTypes.Property {
	seen := make(map[[2]string]bool, len(a))
	result := make([]*altshiftSbomTypes.Property, 0, len(a)+len(b))
	for _, property := range append(slices.Clone(a), b...) {
		if property == nil {
			continue
		}
		key := [2]string{property.Name, property.Value}
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, property)
	}
	return result
}
