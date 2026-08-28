package retry_config

// Config is how a job's failed attempts are retried. The durations are the JSON encoding of a
// protobuf duration -- a number of seconds with an "s" suffix, e.g. "3.5s".
type Config struct {
	// RetryCount is how many times a failed attempt is retried. Zero means the default, which is
	// not the same as never; a job that should not be retried sets it to -1.
	RetryCount int `json:"retryCount,omitzero"`
	// MaxRetryDuration bounds the whole sequence of retries rather than one of them.
	MaxRetryDuration   string `json:"maxRetryDuration,omitzero"`
	MinBackoffDuration string `json:"minBackoffDuration,omitzero"`
	MaxBackoffDuration string `json:"maxBackoffDuration,omitzero"`
	// MaxDoublings is how many times the backoff doubles before it grows linearly instead.
	MaxDoublings int `json:"maxDoublings,omitzero"`
}
