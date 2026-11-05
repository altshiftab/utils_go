package calendar_list_entry

// CalendarListEntry is a calendar as it is on the user's list: their own, one shared with them, or one they
// subscribed to.
type CalendarListEntry struct {
	Id          string `json:"id,omitzero"`
	Summary     string `json:"summary,omitzero"`
	Description string `json:"description,omitzero"`
	// SummaryOverride is the name the user gave the calendar on their list, which they see in place of Summary.
	SummaryOverride string `json:"summaryOverride,omitzero"`
	TimeZone        string `json:"timeZone,omitzero"`
	// AccessRole is what the user may do with it: "owner", "writer", "reader" or "freeBusyReader".
	AccessRole string `json:"accessRole,omitzero"`
	Primary    bool   `json:"primary,omitzero"`
	Selected   bool   `json:"selected,omitzero"`
	Hidden     bool   `json:"hidden,omitzero"`
}
