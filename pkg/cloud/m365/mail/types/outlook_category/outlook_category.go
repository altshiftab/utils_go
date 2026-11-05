package outlook_category

// Category is a category of the mailbox's list: its name, and the colour Outlook shows it in
// ("preset0".."preset24", or "none").
type Category struct {
	Id          string `json:"id,omitzero"`
	DisplayName string `json:"displayName,omitzero"`
	Color       string `json:"color,omitzero"`
}
