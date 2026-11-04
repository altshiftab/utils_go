package user

// User is a Drive user as a file names one: an owner, or whoever changed it last.
type User struct {
	DisplayName  string `json:"displayName,omitzero"`
	EmailAddress string `json:"emailAddress,omitzero"`
	// Me says the user is the one the request is made as.
	Me bool `json:"me,omitzero"`
}
