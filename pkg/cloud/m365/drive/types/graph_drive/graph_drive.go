package graph_drive

// Drive is a OneDrive or a SharePoint document library.
type Drive struct {
	Id        string `json:"id,omitzero"`
	Name      string `json:"name,omitzero"`
	DriveType string `json:"driveType,omitzero"`
	WebUrl    string `json:"webUrl,omitzero"`
}
