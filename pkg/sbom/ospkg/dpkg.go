package ospkg

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
)

// DpkgPackage is one installed Debian package as the dpkg status database records it.
type DpkgPackage struct {
	Name         string
	Version      string
	Architecture string
	// SourceName and SourceVersion identify the source package ("Source: util-linux (2.41-5)"); the version is only
	// given when it differs from the binary package's. Debian's and Ubuntu's advisories are keyed by source package.
	SourceName    string
	SourceVersion string
	Status        string
}

// ParseDpkgStatus reads a dpkg status database (var/lib/dpkg/status, or a file under var/lib/dpkg/status.d as
// distroless images write them): stanzas separated by blank lines, "Field: value" lines with continuation lines
// starting with a space. Only installed packages are returned: a stanza whose Status says so, or one without a Status
// line (status.d files carry none).
func ParseDpkgStatus(data []byte) ([]*DpkgPackage, error) {
	if len(data) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("data"))
	}

	var packages []*DpkgPackage
	var current *DpkgPackage
	flush := func() {
		if current != nil && current.Name != "" && dpkgInstalled(current.Status) {
			packages = append(packages, current)
		}
		current = nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		// Continuation lines belong to multi-line fields (Description, Conffiles) that are not needed.
		if line[0] == ' ' || line[0] == '\t' {
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if current == nil {
			current = &DpkgPackage{}
		}
		switch field {
		case "Package":
			current.Name = value
		case "Version":
			current.Version = value
		case "Architecture":
			current.Architecture = value
		case "Status":
			current.Status = value
		case "Source":
			current.SourceName, current.SourceVersion = parseDpkgSource(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("scanner scan: %w", err))
	}
	flush()

	return packages, nil
}

// dpkgInstalled reads dpkg's status ("install ok installed", "deinstall ok config-files"): a package selected for
// removal or purge is not on the system, anything else (installed, unpacked, half-configured) has its files on disk
// and counts, which is Trivy's rule as well. No status at all means a status.d entry, which only lists installed
// packages.
func dpkgInstalled(status string) bool {
	for field := range strings.FieldsSeq(status) {
		if field == "deinstall" || field == "purge" {
			return false
		}
	}
	return true
}

// parseDpkgSource splits "name (version)" into its parts; the version is optional.
func parseDpkgSource(value string) (string, string) {
	name, rest, ok := strings.Cut(value, "(")
	if !ok {
		return strings.TrimSpace(value), ""
	}
	version, _, _ := strings.Cut(rest, ")")
	return strings.TrimSpace(name), strings.TrimSpace(version)
}
