package ospkg

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
)

// ApkPackage is one installed Alpine package as the apk database records it.
type ApkPackage struct {
	Name    string
	Version string
	Arch    string
	// Origin is the source package (aport) the package was built from, e.g. "busybox" for "ssl_client"; Alpine's
	// security advisories are keyed by it.
	Origin  string
	License string
}

// ParseApkInstalled reads an apk "installed" database (lib/apk/db/installed): records separated by blank lines, each a
// sequence of "X:value" lines where X is a one-letter field.
func ParseApkInstalled(data []byte) ([]*ApkPackage, error) {
	if len(data) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("data"))
	}

	var packages []*ApkPackage
	var current *ApkPackage
	flush := func() {
		if current != nil && current.Name != "" {
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
		if len(line) < 2 || line[1] != ':' {
			continue
		}
		if current == nil {
			current = &ApkPackage{}
		}
		value := line[2:]
		switch line[0] {
		case 'P':
			current.Name = value
		case 'V':
			current.Version = value
		case 'A':
			current.Arch = value
		case 'o':
			current.Origin = value
		case 'L':
			current.License = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("scanner scan: %w", err))
	}
	flush()

	return packages, nil
}
