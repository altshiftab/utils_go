package drive_item

import (
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/types/drive_item/file"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/types/drive_item/folder"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/types/drive_item/item_reference"
)

// DriveItem is a file or folder in a OneDrive or SharePoint document library.
// Exactly one of Folder and File is set.
type DriveItem struct {
	Id                   string                        `json:"id,omitzero"`
	Name                 string                        `json:"name,omitzero"`
	ETag                 string                        `json:"eTag,omitzero"`
	Size                 int64                         `json:"size,omitzero"`
	WebUrl               string                        `json:"webUrl,omitzero"`
	CreatedDateTime      time.Time                     `json:"createdDateTime,omitzero"`
	LastModifiedDateTime time.Time                     `json:"lastModifiedDateTime,omitzero"`
	ParentReference      *item_reference.ItemReference `json:"parentReference,omitzero"`
	Folder               *folder.Folder                `json:"folder,omitzero"`
	File                 *file.File                    `json:"file,omitzero"`
}
