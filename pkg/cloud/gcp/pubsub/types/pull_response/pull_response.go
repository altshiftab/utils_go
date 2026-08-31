package pull_response

import "github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub/types/received_message"

// Response is the result of a subscriptions:pull call. An empty list means no
// message was available within the time the server waited, which is the ordinary
// idle case rather than a failure.
type Response struct {
	ReceivedMessages []*received_message.ReceivedMessage `json:"receivedMessages,omitzero"`
}
