package folder

// Folder is the facet present on a driveItem that is a folder.
type Folder struct {
	ChildCount int `json:"childCount,omitzero"`
}
