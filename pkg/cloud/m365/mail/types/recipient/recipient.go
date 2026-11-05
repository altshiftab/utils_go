package recipient

import "github.com/altshiftab/utils_go/pkg/cloud/m365/mail/types/email_address"

// Recipient is a sender or a recipient of a message.
type Recipient struct {
	EmailAddress *email_address.EmailAddress `json:"emailAddress,omitzero"`
}
