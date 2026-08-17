package bucket

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_storage/types/bucket/lifecycle"
)

type Bucket struct {
	Kind           string               `json:"kind,omitempty"`
	Id             string               `json:"id,omitempty"`
	SelfLink       string               `json:"selfLink,omitempty"`
	Name           string               `json:"name,omitempty"`
	ProjectNumber  string               `json:"projectNumber,omitempty"`
	TimeCreated    string               `json:"timeCreated,omitempty"`
	Updated        string               `json:"updated,omitempty"`
	Location       string               `json:"location,omitempty"`
	LocationType   string               `json:"locationType,omitempty"`
	StorageClass   string               `json:"storageClass,omitempty"`
	Etag           string               `json:"etag,omitempty"`
	Metageneration string               `json:"metageneration,omitempty"`
	Lifecycle      *lifecycle.Lifecycle `json:"lifecycle,omitempty"`
}
