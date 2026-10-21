package watch_response

import (
	"fmt"
	"strconv"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// WatchResponse is what Gmail answers a watch registration with.
type WatchResponse struct {
	// HistoryId is where the mailbox's history stood when the watch was
	// registered. It is the earliest point a push can be read from, history
	// from before a watch not being delivered.
	HistoryId string `json:"historyId,omitzero"`
	// Expiration is when the watch stops, as milliseconds since the epoch in a
	// string, which is how Gmail reports it. ExpiresAt is that as a time.
	Expiration string `json:"expiration,omitzero"`
}

// ExpiresAt is Expiration as a time.
//
// A watch lives seven days and then stops without a word, so when it does is
// the only thing that says when it has to be registered again -- and the field
// carrying it says neither its unit nor its epoch. Every caller of Watch has to
// work that out to use it, so it is worked out here instead.
//
// An empty Expiration is the zero time and no error: Gmail omits it from a
// response that carried no watch, and a caller comparing against now treats a
// zero time as due, which is the right answer.
func (watchResponse *WatchResponse) ExpiresAt() (time.Time, error) {
	if watchResponse == nil || watchResponse.Expiration == "" {
		return time.Time{}, nil
	}

	milliseconds, err := strconv.ParseInt(watchResponse.Expiration, 10, 64)
	if err != nil {
		return time.Time{}, altshiftErrors.NewWithTrace(
			fmt.Errorf("parse int (expiration): %w", err),
			watchResponse.Expiration,
		)
	}

	return time.UnixMilli(milliseconds).UTC(), nil
}
