package event

// The statuses of an event and of an attendee's answer.
const (
	StatusConfirmed = "confirmed"
	StatusTentative = "tentative"
	StatusCancelled = "cancelled"

	ResponseNeedsAction = "needsAction"
	ResponseDeclined    = "declined"
	ResponseTentative   = "tentative"
	ResponseAccepted    = "accepted"
)

// DateTime is when an event starts or ends: Date alone ("2026-11-05") for an event of whole days, DateTime
// (RFC 3339) for one of a time. TimeZone is the IANA zone the time is in, which a recurring event needs to recur by.
type DateTime struct {
	Date     string `json:"date,omitzero"`
	DateTime string `json:"dateTime,omitzero"`
	TimeZone string `json:"timeZone,omitzero"`
}

// Person is an event's creator or organizer.
type Person struct {
	Email       string `json:"email,omitzero"`
	DisplayName string `json:"displayName,omitzero"`
	Self        bool   `json:"self,omitzero"`
}

// Attendee is one of an event's guests, and their answer.
type Attendee struct {
	Email          string `json:"email,omitzero"`
	DisplayName    string `json:"displayName,omitzero"`
	ResponseStatus string `json:"responseStatus,omitzero"`
	Comment        string `json:"comment,omitzero"`
	Optional       bool   `json:"optional,omitzero"`
	Organizer      bool   `json:"organizer,omitzero"`
	// Self says the attendee is the user the request is made as.
	Self     bool `json:"self,omitzero"`
	Resource bool `json:"resource,omitzero"`
	// AdditionalGuests is how many people the attendee brings.
	AdditionalGuests int `json:"additionalGuests,omitzero"`
}

// ConferenceSolutionKey names a kind of conference; "hangoutsMeet" is Google Meet.
type ConferenceSolutionKey struct {
	Type string `json:"type,omitzero"`
}

// CreateConferenceRequest asks for a conference to be made for the event. RequestId is the caller's own, and a
// request repeated with it makes no second one.
type CreateConferenceRequest struct {
	RequestId             string                 `json:"requestId,omitzero"`
	ConferenceSolutionKey *ConferenceSolutionKey `json:"conferenceSolutionKey,omitzero"`
}

// EntryPoint is a way into a conference: a link, a phone number.
type EntryPoint struct {
	EntryPointType string `json:"entryPointType,omitzero"`
	Uri            string `json:"uri,omitzero"`
	Label          string `json:"label,omitzero"`
}

// ConferenceData is the conference an event has, or the one asked to be made for it.
type ConferenceData struct {
	ConferenceId  string                   `json:"conferenceId,omitzero"`
	CreateRequest *CreateConferenceRequest `json:"createRequest,omitzero"`
	EntryPoints   []*EntryPoint            `json:"entryPoints,omitzero"`
}

// Event is an event in a calendar.
//
// Sent as a patch, an event sets the fields it sets and leaves the rest, so a field cannot be emptied by it; a patch
// of a map can (see PatchEvent). A list -- Attendees, Recurrence -- is replaced whole by the one sent.
type Event struct {
	Id          string `json:"id,omitzero"`
	Status      string `json:"status,omitzero"`
	HtmlLink    string `json:"htmlLink,omitzero"`
	Summary     string `json:"summary,omitzero"`
	Description string `json:"description,omitzero"`
	Location    string `json:"location,omitzero"`

	Creator   *Person   `json:"creator,omitzero"`
	Organizer *Person   `json:"organizer,omitzero"`
	Start     *DateTime `json:"start,omitzero"`
	End       *DateTime `json:"end,omitzero"`

	// Recurrence is the RRULE, EXRULE, RDATE and EXDATE lines of a recurring event, as RFC 5545 writes them.
	Recurrence []string `json:"recurrence,omitzero"`
	// RecurringEventId is, for one occurrence of a recurring event, the id of the event it recurs from.
	RecurringEventId string `json:"recurringEventId,omitzero"`

	// Transparency is "opaque" for an event that makes its time busy and "transparent" for one that does not.
	Transparency string `json:"transparency,omitzero"`
	Visibility   string `json:"visibility,omitzero"`
	// EventType is "default", or one of Calendar's own kinds: "outOfOffice", "focusTime", "workingLocation", ...
	EventType string `json:"eventType,omitzero"`

	Attendees []*Attendee `json:"attendees,omitzero"`
	// AttendeesOmitted says Attendees is not every guest: in a patch, that the attendees named are changed and the
	// rest left, which is how a guest answers an invitation without sending back everyone else's.
	AttendeesOmitted bool            `json:"attendeesOmitted,omitzero"`
	HangoutLink      string          `json:"hangoutLink,omitzero"`
	ConferenceData   *ConferenceData `json:"conferenceData,omitzero"`

	Created string `json:"created,omitzero"`
	Updated string `json:"updated,omitzero"`
}
