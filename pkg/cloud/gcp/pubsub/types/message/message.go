package message

import (
	"encoding/base64"
	"fmt"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// Message mirrors the Pub/Sub REST PubsubMessage resource. Note that Data is the
// base64-encoded message payload, as required by the JSON API.
type Message struct {
	Data        string            `json:"data,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
	MessageId   string            `json:"messageId,omitempty"`
	OrderingKey string            `json:"orderingKey,omitempty"`
	PublishTime string            `json:"publishTime,omitempty"`
}

// New constructs a Message from a raw (unencoded) payload, base64-encoding it as the
// REST API expects. Attributes may be nil.
func New(payload []byte, attributes map[string]string) *Message {
	return &Message{
		Data:       base64.StdEncoding.EncodeToString(payload),
		Attributes: attributes,
	}
}

// Payload decodes Data, which the JSON API carries base64-encoded.
//
// A message with no data is not an error: an attributes-only message is a
// legitimate signal, and yields an empty payload rather than a failure.
func (m *Message) Payload() ([]byte, error) {
	if m == nil || m.Data == "" {
		return nil, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(m.Data)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("base64 decode string: %w", err))
	}

	return decoded, nil
}
