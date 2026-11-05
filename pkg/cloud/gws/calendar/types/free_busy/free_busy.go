package free_busy

// Item names a calendar whose busy times are asked for: an address, or a calendar's id.
type Item struct {
	Id string `json:"id"`
}

// Request asks when the calendars are busy between TimeMin and TimeMax (RFC 3339).
type Request struct {
	TimeMin  string  `json:"timeMin"`
	TimeMax  string  `json:"timeMax"`
	TimeZone string  `json:"timeZone,omitzero"`
	Items    []*Item `json:"items"`
}

// Interval is a busy time, its ends in RFC 3339.
type Interval struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Error is why a calendar's busy times could not be told: "notFound", or "internalError".
type Error struct {
	Domain string `json:"domain,omitzero"`
	Reason string `json:"reason,omitzero"`
}

// Calendar is one calendar's busy times, or why there are none to tell.
type Calendar struct {
	Busy   []*Interval `json:"busy,omitzero"`
	Errors []*Error    `json:"errors,omitzero"`
}

// Response is every calendar asked about, by the id it was asked by.
type Response struct {
	TimeMin   string               `json:"timeMin,omitzero"`
	TimeMax   string               `json:"timeMax,omitzero"`
	Calendars map[string]*Calendar `json:"calendars,omitzero"`
}
