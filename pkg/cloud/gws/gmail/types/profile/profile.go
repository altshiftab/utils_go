// Package profile is a Gmail mailbox's own account of itself.
package profile

// Profile is what users.getProfile returns.
//
// HistoryId is the reason to ask for it: it is where the mailbox's history
// stands now, and nothing else reports that. A message's history id says when
// that message was last touched, and a history listing can only be read from a
// cursor that is still inside Gmail's window -- so a caller whose cursor has
// gone stale has this and nothing else to start again from.
type Profile struct {
	EmailAddress string `json:"emailAddress,omitzero"`
	// MessagesTotal and ThreadsTotal count everything in the mailbox, spam and
	// trash included.
	MessagesTotal int `json:"messagesTotal,omitzero"`
	ThreadsTotal  int `json:"threadsTotal,omitzero"`
	// HistoryId is the mailbox's current history record id, as a decimal
	// string.
	HistoryId string `json:"historyId,omitzero"`
}
