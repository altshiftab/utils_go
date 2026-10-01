package site

// Site is a SharePoint site.
type Site struct {
	// Id is the composite "hostname,site-collection-id,web-id".
	Id          string `json:"id,omitzero"`
	Name        string `json:"name,omitzero"`
	DisplayName string `json:"displayName,omitzero"`
	WebUrl      string `json:"webUrl,omitzero"`
}
