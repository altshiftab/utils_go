package acknowledge_request

// Request is the body of a subscriptions:acknowledge call.
type Request struct {
	AckIds []string `json:"ackIds"`
}
