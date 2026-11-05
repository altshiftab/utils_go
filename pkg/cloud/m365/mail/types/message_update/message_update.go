package message_update

// Update is what is changed of a message. A nil field is left as it is; Categories set to an empty
// slice takes every category off.
type Update struct {
	Categories []string `json:"categories,omitzero"`
	IsRead     *bool    `json:"isRead,omitzero"`
}
