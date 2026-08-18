// Package image analyzes container images: it reads the files that record what an image holds — the OS package
// databases, os-release, Go binaries' build info, shipped node packages — and hands back plain findings. The image is
// read as a docker-archive stream, obtained from podman or from any reader.
package image

import (
	"archive/tar"
	"bufio"
	"bytes"
	"debug/buildinfo"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// ErrFileTooLarge reports a package database beyond what is kept in memory.
var ErrFileTooLarge = errors.New("file too large")

const (
	// maxTextFileSize bounds the package databases and other text files kept in memory.
	maxTextFileSize = 256 << 20
	// maxInMemoryBinarySize bounds the executables read into memory for build info; larger ones are spooled to disk.
	maxInMemoryBinarySize = 64 << 20
	// minElfSize is the size of an ELF header; anything shorter cannot be an executable.
	minElfSize = 64
)

// fileKind classifies the paths the analysis captures.
type fileKind int

const (
	kindOther fileKind = iota
	kindOsRelease
	kindApkDatabase
	kindDpkgStatus
	kindRpmDatabase
	kindNodePackage
)

var (
	osReleasePaths   = []string{"etc/os-release", "usr/lib/os-release"}
	apkDatabasePaths = []string{"lib/apk/db/installed", "usr/lib/apk/db/installed"}
	dpkgStatusPath   = "var/lib/dpkg/status"
	dpkgStatusDir    = "var/lib/dpkg/status.d"
	rpmDatabasePaths = []string{
		"var/lib/rpm/Packages",
		"var/lib/rpm/Packages.db",
		"var/lib/rpm/rpmdb.sqlite",
		"usr/lib/sysimage/rpm/Packages",
		"usr/lib/sysimage/rpm/Packages.db",
		"usr/lib/sysimage/rpm/rpmdb.sqlite",
	}
	elfMagic = []byte{0x7f, 'E', 'L', 'F'}
)

// GoBinary is an executable built by Go, with the modules embedded in it.
type GoBinary struct {
	// Path is the path inside the image, without a leading slash.
	Path string
	// Layer is the diff ID of the layer that last wrote the executable.
	Layer string
	Info  *buildinfo.BuildInfo
}

// NodePackage is a package.json found under a node_modules directory.
type NodePackage struct {
	Path string
	// Layer is the diff ID of the layer that last wrote the manifest.
	Layer   string
	Name    string
	Version string
	License string
}

// textPayload is the content of a captured text file.
type textPayload []byte

// rpmMarker records that an RPM database exists; its contents are not read.
type rpmMarker struct{}

// classify tells which of the captured kinds a path is.
func classify(filePath string) fileKind {
	switch {
	case contains(osReleasePaths, filePath):
		return kindOsRelease
	case contains(apkDatabasePaths, filePath):
		return kindApkDatabase
	case filePath == dpkgStatusPath:
		return kindDpkgStatus
	case contains(rpmDatabasePaths, filePath):
		return kindRpmDatabase
	}

	if dir, base := path.Split(filePath); strings.TrimSuffix(dir, "/") == dpkgStatusDir && !strings.HasSuffix(base, ".md5sums") {
		return kindDpkgStatus
	}

	if isNodePackageJson(filePath) {
		return kindNodePackage
	}

	return kindOther
}

// isNodePackageJson matches "<...>/node_modules/<name>/package.json" and
// "<...>/node_modules/@<scope>/<name>/package.json", the manifests of installed node packages.
func isNodePackageJson(filePath string) bool {
	segments := strings.Split(filePath, "/")
	if len(segments) < 3 || segments[len(segments)-1] != "package.json" {
		return false
	}
	rest := segments[:len(segments)-1]
	// The last node_modules segment decides; a package's own nested node_modules count separately.
	for i := len(rest) - 1; i >= 0; i-- {
		if rest[i] != "node_modules" {
			continue
		}
		after := rest[i+1:]
		switch len(after) {
		case 1:
			return after[0] != "" && !strings.HasPrefix(after[0], "@") && !strings.HasPrefix(after[0], ".")
		case 2:
			return strings.HasPrefix(after[0], "@") && after[1] != "" && !strings.HasPrefix(after[1], ".")
		default:
			return false
		}
	}
	return false
}

func contains(paths []string, filePath string) bool {
	for _, candidate := range paths {
		if candidate == filePath {
			return true
		}
	}
	return false
}

// capture is the policy handed to the archive reader: package databases and os-release are kept as text, executables
// are read for Go build info, node package manifests are parsed, RPM databases only leave a marker.
func capture(filePath string, header *tar.Header, reader *bufio.Reader) (any, error) {
	switch classify(filePath) {
	case kindOsRelease, kindApkDatabase, kindDpkgStatus:
		if header.Size > maxTextFileSize {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %s (%d bytes)", ErrFileTooLarge, filePath, header.Size), filePath, header.Size)
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("read all: %w", err), filePath)
		}
		return textPayload(data), nil
	case kindRpmDatabase:
		return rpmMarker{}, nil
	case kindNodePackage:
		if header.Size > maxTextFileSize {
			return nil, nil
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("read all: %w", err), filePath)
		}
		// A typed nil must not become a non-nil payload.
		if nodePackage := parseNodePackageJson(filePath, data); nodePackage != nil {
			return nodePackage, nil
		}
		return nil, nil
	default:
		return captureBuildInfo(filePath, header, reader)
	}
}

// goBuildInfo tells a captured build-info payload apart from the others.
func goBuildInfo(payload any) (*buildinfo.BuildInfo, bool) {
	info, ok := payload.(*buildinfo.BuildInfo)
	return info, ok && info != nil
}

// captureBuildInfo returns the Go build info of an executable, or nil for anything that is not a Go executable.
func captureBuildInfo(filePath string, header *tar.Header, reader *bufio.Reader) (any, error) {
	if header.Size < minElfSize {
		return nil, nil
	}
	magic, err := reader.Peek(len(elfMagic))
	if err != nil || !bytes.Equal(magic, elfMagic) {
		return nil, nil
	}

	var readerAt io.ReaderAt
	if header.Size <= maxInMemoryBinarySize {
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("read all: %w", err), filePath)
		}
		readerAt = bytes.NewReader(data)
	} else {
		file, err := os.CreateTemp("", "sbom-image-*")
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("os create temp: %w", err))
		}
		defer func() {
			_ = file.Close()
			_ = os.Remove(file.Name())
		}()
		if _, err := io.Copy(file, reader); err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("io copy: %w", err), filePath)
		}
		readerAt = file
	}

	// Anything buildinfo cannot read is not a Go executable (or is a stripped one); either way there is nothing
	// to list.
	info, err := buildinfo.Read(readerAt)
	if err != nil {
		return nil, nil
	}
	return info, nil
}

type nodePackageJson struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	License any    `json:"license"`
}

// parseNodePackageJson reads the name, version and license of a package.json; a manifest without a name and version
// yields nil.
func parseNodePackageJson(filePath string, data []byte) *NodePackage {
	var manifest nodePackageJson
	if err := json.Unmarshal(data, &manifest, json.MatchCaseInsensitiveNames(false)); err != nil {
		return nil
	}
	if manifest.Name == "" || manifest.Version == "" {
		return nil
	}

	var license string
	switch value := manifest.License.(type) {
	case string:
		license = value
	case map[string]any:
		license, _ = value["type"].(string)
	}

	return &NodePackage{Path: filePath, Name: manifest.Name, Version: manifest.Version, License: license}
}
