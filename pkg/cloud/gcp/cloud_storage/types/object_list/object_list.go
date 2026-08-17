package object_list

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_storage/types/object"
)

type ObjectList struct {
	Kind          string           `json:"kind,omitempty"`
	Items         []*object.Object `json:"items,omitempty"`
	Prefixes      []string         `json:"prefixes,omitempty"`
	NextPageToken string           `json:"nextPageToken,omitempty"`
}
