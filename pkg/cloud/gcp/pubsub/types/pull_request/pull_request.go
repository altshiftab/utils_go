package pull_request

// Request is the body of a subscriptions:pull call.
//
// The call is synchronous, but it is not a poll in the busy sense: the server
// holds the request briefly while it waits for messages rather than returning an
// empty response at once, so a loop over it behaves like long polling. It is the
// simpler half of the two pull mechanisms -- the other, StreamingPull, is a
// bidirectional gRPC stream, which delivers with less latency at the cost of
// gRPC and its generated code.
type Request struct {
	// MaxMessages bounds one response. The server may return fewer, including
	// none, and returning none is not an error.
	MaxMessages int `json:"maxMessages"`
}
