package modify_request

// Request changes which labels a message or thread carries.
//
// Both fields are optional and a request giving neither is accepted by Gmail
// and changes nothing, which is why the client refuses it instead: a modify
// that names no label is a caller that meant something it did not say.
//
// Labels are named by id rather than by display name. The ids of the built-in
// ones are their names in upper case -- INBOX, UNREAD, STARRED -- while a label
// somebody created has an opaque id that has to be looked up.
type Request struct {
	AddLabelIds    []string `json:"addLabelIds,omitzero"`
	RemoveLabelIds []string `json:"removeLabelIds,omitzero"`
}
