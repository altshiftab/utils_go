package history

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message"
)

type MessageChange struct {
	Message *message.Message `json:"message,omitzero"`
}

type LabelChange struct {
	Message  *message.Message `json:"message,omitzero"`
	LabelIds []string         `json:"labelIds,omitzero"`
}

type Record struct {
	Id              string             `json:"id,omitzero"`
	Messages        []*message.Message `json:"messages,omitzero"`
	MessagesAdded   []*MessageChange   `json:"messagesAdded,omitzero"`
	MessagesDeleted []*MessageChange   `json:"messagesDeleted,omitzero"`
	LabelsAdded     []*LabelChange     `json:"labelsAdded,omitzero"`
	LabelsRemoved   []*LabelChange     `json:"labelsRemoved,omitzero"`
}
