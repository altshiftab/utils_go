package image_reference

import (
	"fmt"
	"strings"

	"github.com/altshiftab/utils_go/pkg/container/types/image_reference"
	"github.com/altshiftab/utils_go/pkg/schema"
)

const dockerPkgDevSuffix = "-docker.pkg.dev"

type Reference struct {
	image_reference.Reference
	Region    string `json:"region"`
	ProjectId string `json:"project_id"`
}

func (r *Reference) Cloud() *schema.Cloud {
	var cloudProject *schema.CloudProject
	if r.ProjectId != "" {
		cloudProject = &schema.CloudProject{Id: r.ProjectId}
	}

	if r.Region == "" && cloudProject == nil {
		return nil
	}

	return &schema.Cloud{Region: r.Region, Project: cloudProject}
}

func Parse(data string) (*Reference, error) {
	baseImageReference, err := image_reference.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("image reference parse: %w", err)
	}

	reference := &Reference{
		Reference: *baseImageReference,
	}

	if strings.HasSuffix(baseImageReference.Registry, dockerPkgDevSuffix) {
		reference.Region = strings.TrimSuffix(baseImageReference.Registry, dockerPkgDevSuffix)
	}

	if slashIdx := strings.Index(baseImageReference.Repository, "/"); slashIdx != -1 {
		reference.ProjectId = baseImageReference.Repository[:slashIdx]
	} else {
		reference.ProjectId = baseImageReference.Repository
	}

	return reference, nil
}
