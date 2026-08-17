package manifest

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/artifact_registry/types/descriptor"
)

type Manifest struct {
	SchemaVersion int                      `json:"schemaVersion,omitempty"`
	MediaType     string                   `json:"mediaType,omitempty"`
	ArtifactType  string                   `json:"artifactType,omitempty"`
	Config        *descriptor.Descriptor   `json:"config,omitempty"`
	Layers        []*descriptor.Descriptor `json:"layers,omitempty"`
	Subject       *descriptor.Descriptor   `json:"subject,omitempty"`
	Annotations   map[string]string        `json:"annotations,omitempty"`
}
