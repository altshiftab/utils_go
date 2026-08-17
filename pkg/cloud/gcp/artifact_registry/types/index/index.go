package index

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/artifact_registry/types/descriptor"
)

type Index struct {
	SchemaVersion int                      `json:"schemaVersion,omitempty"`
	MediaType     string                   `json:"mediaType,omitempty"`
	Manifests     []*descriptor.Descriptor `json:"manifests,omitempty"`
	Annotations   map[string]string        `json:"annotations,omitempty"`
}
