package message_part

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message/message_part/body"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message/message_part/header"
)

type MessagePart struct {
	PartId   string           `json:"partId,omitzero"`
	MimeType string           `json:"mimeType,omitzero"`
	Filename string           `json:"filename,omitzero"`
	Headers  []*header.Header `json:"headers,omitzero"`
	Body     *body.Body       `json:"body,omitzero"`
	Parts    []*MessagePart   `json:"parts,omitzero"`
}
