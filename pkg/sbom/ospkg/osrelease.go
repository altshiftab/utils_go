// Package ospkg parses the files that record which operating-system packages an image or a system holds — the apk
// installed database, the dpkg status database and os-release — into plain structs. Which package URLs those become
// is decided by the callers.
package ospkg

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
)

// OsRelease holds the fields of os-release(5) that identify a distribution.
type OsRelease struct {
	Id              string
	IdLike          string
	Name            string
	PrettyName      string
	VersionId       string
	VersionCodename string
}

// ParseOsRelease reads an os-release file: KEY=value lines, values optionally quoted with single or double quotes,
// blank lines and # comments ignored.
func ParseOsRelease(data []byte) (*OsRelease, error) {
	if len(data) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("data"))
	}

	osRelease := &OsRelease{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = unquoteOsReleaseValue(strings.TrimSpace(value))

		switch key {
		case "ID":
			osRelease.Id = value
		case "ID_LIKE":
			osRelease.IdLike = value
		case "NAME":
			osRelease.Name = value
		case "PRETTY_NAME":
			osRelease.PrettyName = value
		case "VERSION_ID":
			osRelease.VersionId = value
		case "VERSION_CODENAME":
			osRelease.VersionCodename = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("scanner scan: %w", err))
	}

	return osRelease, nil
}

// unquoteOsReleaseValue strips one level of matching quotes and, inside double quotes, the backslash escapes the spec
// allows (\", \$, \`, \\).
func unquoteOsReleaseValue(value string) string {
	if len(value) < 2 {
		return value
	}
	quote := value[0]
	if (quote != '"' && quote != '\'') || value[len(value)-1] != quote {
		return value
	}
	value = value[1 : len(value)-1]
	if quote == '\'' {
		return value
	}

	var builder strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) {
			switch value[i+1] {
			case '"', '$', '`', '\\':
				i++
			}
		}
		builder.WriteByte(value[i])
	}
	return builder.String()
}
