package draft

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message"
)

// Draft is a message saved and not yet sent.
//
// Its id is the draft's own and outlives an edit, while the message under it is
// replaced by every update and gets a new id each time: a caller holding on to a
// draft names it by Id, never by Message.Id.
type Draft struct {
	Id      string           `json:"id,omitzero"`
	Message *message.Message `json:"message,omitzero"`
}
