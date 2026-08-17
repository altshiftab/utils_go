package image_reference

import (
	"strings"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/missing_error"
	"github.com/altshiftab/utils_go/pkg/schema"
)

type Reference struct {
	Registry   string
	Repository string
	Tag        string
	Digest     string
}

func (r *Reference) ContainerImage() *schema.ContainerImage {
	var name string
	if r.Registry != "" && r.Repository != "" {
		name = r.Registry + "/" + r.Repository
	}

	var imageHash *schema.ContainerImageHash
	if r.Digest != "" {
		imageHash = &schema.ContainerImageHash{All: []string{r.Digest}}
	}

	if name == "" && imageHash == nil {
		return nil
	}

	return &schema.ContainerImage{Name: name, Tag: r.Tag, Hash: imageHash}
}

func Parse(data string) (*Reference, error) {
	reference := &Reference{}

	if before, digest, found := strings.Cut(data, "@"); found {
		reference.Digest = digest
		data = before
	}

	if colonIdx := strings.LastIndex(data, ":"); colonIdx != -1 {
		afterColon := data[colonIdx+1:]
		if !strings.Contains(afterColon, "/") {
			reference.Tag = afterColon
			data = data[:colonIdx]
		}
	}

	registry, repository, found := strings.Cut(data, "/")
	if !found {
		return nil, motmedelErrors.NewWithTrace(missing_error.New("registry"))
	}
	reference.Registry = registry
	reference.Repository = repository

	if reference.Repository == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("repository"))
	}

	return reference, nil
}
