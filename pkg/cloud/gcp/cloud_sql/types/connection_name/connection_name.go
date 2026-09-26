package connection_name

import (
	"fmt"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
)

// ConnectionName identifies a Cloud SQL instance as `project:region:instance`. A domain-scoped
// project (`example.com:project`) carries its own colon, which the project keeps.
type ConnectionName struct {
	Project  string
	Region   string
	Instance string
}

func (c *ConnectionName) String() string {
	return c.Project + ":" + c.Region + ":" + c.Instance
}

func Parse(s string) (*ConnectionName, error) {
	if s == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("connection name"))
	}

	parts := strings.Split(s, ":")
	switch len(parts) {
	case 3:
	case 4:
		parts = []string{parts[0] + ":" + parts[1], parts[2], parts[3]}
	default:
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: connection name is not project:region:instance", altshiftErrors.ErrParseError),
			s,
		)
	}

	for i, name := range []string{"project", "region", "instance"} {
		if parts[i] == "" {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %w", altshiftErrors.ErrParseError, empty_error.New(name)),
				s,
			)
		}
	}

	return &ConnectionName{Project: parts[0], Region: parts[1], Instance: parts[2]}, nil
}
