package item_reference

type ItemReference struct {
	DriveId   string `json:"driveId,omitzero"`
	DriveType string `json:"driveType,omitzero"`
	Id        string `json:"id,omitzero"`
	Name      string `json:"name,omitzero"`
	// Path is percent-encoded and relative to the drive, e.g. "/drive/root:/Reports".
	Path   string `json:"path,omitzero"`
	SiteId string `json:"siteId,omitzero"`
}
