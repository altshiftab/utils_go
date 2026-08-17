package message

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message/message_part"
)

type Message struct {
	Id           string   `json:"id,omitzero"`
	ThreadId     string   `json:"threadId,omitzero"`
	LabelIds     []string `json:"labelIds,omitzero"`
	Snippet      string   `json:"snippet,omitzero"`
	HistoryId    string   `json:"historyId,omitzero"`
	InternalDate string   `json:"internalDate,omitzero"`
	SizeEstimate int      `json:"sizeEstimate,omitzero"`
	Raw          string   `json:"raw,omitzero"`

	Payload *message_part.MessagePart `json:"payload,omitzero"`
}
