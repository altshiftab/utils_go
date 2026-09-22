package label

// Label is one of a mailbox's labels.
//
// Id is what every other call names a label by, and it is not the name: a
// user-created label has an opaque id, so applying one means listing or
// creating first to learn it.
type Label struct {
	Id   string `json:"id,omitzero"`
	Name string `json:"name,omitzero"`
	// Type is "system" for the built-in labels and "user" for the rest. A
	// system label cannot be created, renamed or deleted.
	Type string `json:"type,omitzero"`

	// MessageListVisibility is "show" or "hide": whether a message carrying
	// this label shows it in the message list.
	MessageListVisibility string `json:"messageListVisibility,omitzero"`
	// LabelListVisibility is "labelShow", "labelShowIfUnread" or "labelHide":
	// whether the label itself appears in the sidebar.
	LabelListVisibility string `json:"labelListVisibility,omitzero"`
}
