package event_list

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gws/calendar/types/event"
)

// EventList is one page of a calendar's events.
type EventList struct {
	Items         []*event.Event `json:"items,omitzero"`
	NextPageToken string         `json:"nextPageToken,omitzero"`
	// TimeZone is the calendar's own, which the times of its events are in unless the listing asked for another.
	TimeZone string `json:"timeZone,omitzero"`
	Summary  string `json:"summary,omitzero"`
}
