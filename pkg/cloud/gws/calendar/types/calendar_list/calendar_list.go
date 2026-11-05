package calendar_list

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gws/calendar/types/calendar_list_entry"
)

// CalendarList is one page of the user's list of calendars.
type CalendarList struct {
	Items         []*calendar_list_entry.CalendarListEntry `json:"items,omitzero"`
	NextPageToken string                                   `json:"nextPageToken,omitzero"`
}
