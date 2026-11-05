package email_address

// EmailAddress is an address with the name shown for it.
type EmailAddress struct {
	Name    string `json:"name,omitzero"`
	Address string `json:"address,omitzero"`
}
