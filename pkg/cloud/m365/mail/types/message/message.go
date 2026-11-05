package message

import (
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/m365/mail/types/item_body"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/mail/types/recipient"
)

// The kinds of attachment, by their @odata.type.
const (
	// AttachmentTypeFile is a file, whose bytes are the attachment's content.
	AttachmentTypeFile = "#microsoft.graph.fileAttachment"
	// AttachmentTypeItem is a message, an event or a contact attached whole; its content is MIME.
	AttachmentTypeItem = "#microsoft.graph.itemAttachment"
	// AttachmentTypeReference is a link to a file kept in OneDrive or SharePoint, which has no content.
	AttachmentTypeReference = "#microsoft.graph.referenceAttachment"
)

// Message is a message as Graph serves it; what is filled in is what was selected.
type Message struct {
	Id                string              `json:"id,omitzero"`
	ConversationId    string              `json:"conversationId,omitzero"`
	ParentFolderId    string              `json:"parentFolderId,omitzero"`
	InternetMessageId string              `json:"internetMessageId,omitzero"`
	Subject           string              `json:"subject,omitzero"`
	BodyPreview       string              `json:"bodyPreview,omitzero"`
	Body              *item_body.ItemBody `json:"body,omitzero"`
	// UniqueBody is the part of the body that is not quoted from earlier messages of the conversation.
	UniqueBody       *item_body.ItemBody    `json:"uniqueBody,omitzero"`
	From             *recipient.Recipient   `json:"from,omitzero"`
	Sender           *recipient.Recipient   `json:"sender,omitzero"`
	ToRecipients     []*recipient.Recipient `json:"toRecipients,omitzero"`
	CcRecipients     []*recipient.Recipient `json:"ccRecipients,omitzero"`
	BccRecipients    []*recipient.Recipient `json:"bccRecipients,omitzero"`
	ReplyTo          []*recipient.Recipient `json:"replyTo,omitzero"`
	ReceivedDateTime time.Time              `json:"receivedDateTime,omitzero"`
	SentDateTime     time.Time              `json:"sentDateTime,omitzero"`
	HasAttachments   bool                   `json:"hasAttachments,omitzero"`
	IsRead           bool                   `json:"isRead,omitzero"`
	IsDraft          bool                   `json:"isDraft,omitzero"`
	Importance       string                 `json:"importance,omitzero"`
	Categories       []string               `json:"categories,omitzero"`
	// WebLink opens the message in Outlook on the web.
	WebLink     string        `json:"webLink,omitzero"`
	Attachments []*Attachment `json:"attachments,omitzero"`
}

// Attachment is an attachment of a message, of any kind; OdataType says which. It lives beside
// Message because an item attachment holds a message in turn.
type Attachment struct {
	OdataType            string    `json:"@odata.type,omitzero"`
	Id                   string    `json:"id,omitzero"`
	Name                 string    `json:"name,omitzero"`
	ContentType          string    `json:"contentType,omitzero"`
	Size                 int64     `json:"size,omitzero"`
	IsInline             bool      `json:"isInline,omitzero"`
	LastModifiedDateTime time.Time `json:"lastModifiedDateTime,omitzero"`
	// ContentId is what an inline file is referred to by from the body (cid:).
	ContentId string `json:"contentId,omitzero"`
	// ContentBytes is a file's content, where it was selected.
	ContentBytes []byte `json:"contentBytes,omitzero"`
	// Item is an item attachment's message, where it was expanded.
	Item *Message `json:"item,omitzero"`
}
