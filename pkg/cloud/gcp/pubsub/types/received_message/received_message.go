package received_message

import "github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub/types/message"

// ReceivedMessage mirrors the Pub/Sub REST ReceivedMessage resource: a message
// together with the handle used to acknowledge it.
//
// The handle is what makes delivery at-least-once rather than at-most-once. A
// message stays outstanding until it is acknowledged, and is delivered again if
// the acknowledgement deadline passes -- so a consumer that acknowledges before
// it has finished with a message can lose it, and one that crashes will see it
// twice. Neither is a defect to design around; the second is the one to prefer.
type ReceivedMessage struct {
	AckId   string           `json:"ackId"`
	Message *message.Message `json:"message,omitzero"`
	// DeliveryAttempt counts deliveries when the subscription has a dead letter
	// policy, and is absent otherwise.
	DeliveryAttempt int `json:"deliveryAttempt,omitzero"`
}
