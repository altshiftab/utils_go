package thread

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message"
)

// Thread is a conversation: the messages Gmail considers part of one exchange.
//
// Messages is populated by a read of the thread and left empty by a call that
// only changes it, so a caller should not read the absence as an empty
// conversation.
type Thread struct {
	Id        string             `json:"id,omitzero"`
	Snippet   string             `json:"snippet,omitzero"`
	HistoryId string             `json:"historyId,omitzero"`
	Messages  []*message.Message `json:"messages,omitzero"`
}
