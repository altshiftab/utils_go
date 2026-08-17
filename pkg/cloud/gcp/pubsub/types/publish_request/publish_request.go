package publish_request

import "github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub/types/message"

// Request is the body of a topics:publish call.
type Request struct {
	Messages []*message.Message `json:"messages"`
}
