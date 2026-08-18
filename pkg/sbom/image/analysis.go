package image

import (
	"fmt"
	"io"
	"slices"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/sbom/image/dockerarchive"
	"github.com/altshiftab/utils_go/pkg/sbom/ospkg"
)

// Analysis is what was found in an image. Every finding names the layer (by diff ID) that last wrote the file it was
// read from: for a package database that is the layer that installed, or last changed, the packages it lists.
type Analysis struct {
	// Reference is the name the image was analyzed under.
	Reference string
	// Id is the image ID ("sha256:...").
	Id       string
	RepoTags []string
	// Layers are the image's layer diff IDs, lowest first.
	Layers []string

	OsRelease      *ospkg.OsRelease
	OsReleasePath  string
	OsReleaseLayer string

	ApkPackages      []*ospkg.ApkPackage
	ApkDatabasePath  string
	ApkDatabaseLayer string

	// DpkgStatuses are the dpkg status files found (var/lib/dpkg/status and the status.d entries), each with the
	// installed packages it lists.
	DpkgStatuses []*DpkgStatus

	GoBinaries   []*GoBinary
	NodePackages []*NodePackage

	// Warnings tell what the analysis saw but could not list, e.g. an RPM database.
	Warnings []string
}

// DpkgStatus is one dpkg status file and the installed packages it lists.
type DpkgStatus struct {
	Path     string
	Layer    string
	Packages []*ospkg.DpkgPackage
}

// DpkgPackages lists the installed packages of all dpkg status files, in file order.
func (analysis *Analysis) DpkgPackages() []*ospkg.DpkgPackage {
	var packages []*ospkg.DpkgPackage
	for _, status := range analysis.DpkgStatuses {
		if status != nil {
			packages = append(packages, status.Packages...)
		}
	}
	return packages
}

// AnalyzeArchive analyzes the image a docker-archive stream holds (see dockerarchive.Read for the reference).
func AnalyzeArchive(reader io.Reader, reference string) (*Analysis, error) {
	if reader == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("reader"))
	}

	image, err := dockerarchive.Read(reader, reference, capture)
	if err != nil {
		return nil, fmt.Errorf("docker archive read: %w", err)
	}
	if image == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("image"))
	}

	analysis := &Analysis{Reference: reference, Id: image.Id, RepoTags: image.RepoTags, Layers: image.LayerDigests}

	// Files are visited in path order so that the result does not depend on map iteration.
	paths := make([]string, 0, len(image.Files))
	for filePath := range image.Files {
		paths = append(paths, filePath)
	}
	slices.Sort(paths)

	for _, filePath := range paths {
		file := image.Files[filePath]
		if file == nil {
			continue
		}
		layer := image.LayerDigest(file.Layer)
		switch payload := file.Payload.(type) {
		case textPayload:
			if err := analysis.addText(filePath, layer, payload); err != nil {
				return nil, err
			}
		case rpmMarker:
			analysis.Warnings = append(analysis.Warnings, fmt.Sprintf("rpm database present at /%s; RPM packages are not listed", filePath))
		case *NodePackage:
			payload.Layer = layer
			analysis.NodePackages = append(analysis.NodePackages, payload)
		default:
			// The capture stores build info as-is; wrap it with its path and layer here.
			if info, ok := goBuildInfo(payload); ok {
				analysis.GoBinaries = append(analysis.GoBinaries, &GoBinary{Path: filePath, Layer: layer, Info: info})
			}
		}
	}

	if analysis.OsRelease == nil && (analysis.ApkDatabasePath != "" || len(analysis.DpkgStatuses) != 0) {
		analysis.Warnings = append(analysis.Warnings, "no os-release found; the distribution of the OS packages is unknown")
	}

	return analysis, nil
}

// addText parses a captured text file into the analysis, preferring the canonical location when a file exists in
// several ("etc/os-release" over "usr/lib/os-release", "lib/apk/db/installed" over "usr/lib/apk/db/installed").
func (analysis *Analysis) addText(filePath, layer string, data textPayload) error {
	switch classify(filePath) {
	case kindOsRelease:
		if analysis.OsReleasePath != "" && preferred(osReleasePaths, analysis.OsReleasePath, filePath) {
			return nil
		}
		osRelease, err := ospkg.ParseOsRelease(data)
		if err != nil {
			return altshiftErrors.New(fmt.Errorf("parse os release (%s): %w", filePath, err), filePath)
		}
		analysis.OsRelease, analysis.OsReleasePath, analysis.OsReleaseLayer = osRelease, filePath, layer
	case kindApkDatabase:
		if analysis.ApkDatabasePath != "" && preferred(apkDatabasePaths, analysis.ApkDatabasePath, filePath) {
			return nil
		}
		packages, err := ospkg.ParseApkInstalled(data)
		if err != nil {
			return altshiftErrors.New(fmt.Errorf("parse apk installed (%s): %w", filePath, err), filePath)
		}
		analysis.ApkPackages, analysis.ApkDatabasePath, analysis.ApkDatabaseLayer = packages, filePath, layer
	case kindDpkgStatus:
		if len(strings.TrimSpace(string(data))) == 0 {
			return nil
		}
		packages, err := ospkg.ParseDpkgStatus(data)
		if err != nil {
			return altshiftErrors.New(fmt.Errorf("parse dpkg status (%s): %w", filePath, err), filePath)
		}
		analysis.DpkgStatuses = append(analysis.DpkgStatuses, &DpkgStatus{Path: filePath, Layer: layer, Packages: packages})
	case kindOther, kindRpmDatabase, kindNodePackage:
		// Not text the analysis reads.
	}
	return nil
}

// preferred tells whether the path already chosen ranks before the candidate in the given preference order.
func preferred(order []string, chosen, candidate string) bool {
	return slices.Index(order, chosen) <= slices.Index(order, candidate)
}
