// Package calendar is a client of the Google Calendar API: the user's calendars, their events, and when people are
// busy.
package calendar

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/internal/rest"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	"github.com/altshiftab/utils_go/pkg/utils"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/calendar/calendar_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/calendar/list_events_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/calendar/types/calendar_list"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/calendar/types/event"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/calendar/types/event_list"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/calendar/types/free_busy"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/calendar/write_event_config"
)

const Domain = "www.googleapis.com"

const (
	// ScopeEvents reads and writes the events of every calendar the user can, and not the calendars themselves.
	ScopeEvents = "https://www.googleapis.com/auth/calendar.events"
	// ScopeReadonly reads every calendar the user can, its list and settings among them.
	ScopeReadonly = "https://www.googleapis.com/auth/calendar.readonly"
	// Scope is all of Calendar, sharing a calendar with someone among it.
	Scope = "https://www.googleapis.com/auth/calendar"
)

// Primary is the id of the user's own main calendar.
const Primary = "primary"

var defaultBaseUrl = &url.URL{
	Scheme: "https",
	Host:   Domain,
}

type Client struct {
	baseUrl *url.URL
	config  *calendar_config.Config
}

func NewClient(options ...calendar_config.Option) *Client {
	config := calendar_config.New(options...)
	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = defaultBaseUrl
	}
	u := *baseUrl
	u.Path = "/calendar/v3/"
	return &Client{baseUrl: &u, config: config}
}

func (c *Client) urlString(query url.Values, segments ...string) string {
	return rest.Url(c.baseUrl, query, segments...)
}

func (c *Client) fetchOptions(options []fetch_config.Option) []fetch_config.Option {
	return append(c.config.FetchOptions, options...)
}

func eventsPath(calendarId string) []string {
	return []string{"calendars", calendarId, "events"}
}

func eventPath(calendarId string, eventId string) []string {
	return append(eventsPath(calendarId), eventId)
}

// writeQuery is the query of a call that makes, changes or deletes an event.
func writeQuery(config *write_event_config.Config) url.Values {
	query := url.Values{}
	if config.SendUpdates != "" {
		query.Set("sendUpdates", config.SendUpdates)
	}
	if config.ConferenceDataVersion != 0 {
		query.Set("conferenceDataVersion", strconv.Itoa(config.ConferenceDataVersion))
	}
	return query
}

// ListCalendarsPage retrieves one page of the user's list of calendars, beginning at the page token.
func (c *Client) ListCalendarsPage(ctx context.Context, pageToken string, options ...fetch_config.Option) (*calendar_list.CalendarList, error) {
	query := url.Values{}
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	return rest.GetJson[calendar_list.CalendarList](ctx, c.urlString(query, "users", "me", "calendarList"), c.fetchOptions(options))
}

// ListEventsPage retrieves one page of the calendar's events matching the options, beginning at the options' page
// token.
func (c *Client) ListEventsPage(
	ctx context.Context,
	calendarId string,
	options ...list_events_config.Option,
) (*event_list.EventList, error) {
	if calendarId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("calendar id"))
	}

	listConfig := list_events_config.New(options...)

	query := url.Values{}
	if !listConfig.TimeMin.IsZero() {
		query.Set("timeMin", listConfig.TimeMin.Format(time.RFC3339))
	}
	if !listConfig.TimeMax.IsZero() {
		query.Set("timeMax", listConfig.TimeMax.Format(time.RFC3339))
	}
	if listConfig.Query != "" {
		query.Set("q", listConfig.Query)
	}
	if listConfig.SingleEvents {
		query.Set("singleEvents", "true")
	}
	if listConfig.OrderBy != "" {
		query.Set("orderBy", listConfig.OrderBy)
	}
	if listConfig.MaxResults > 0 {
		query.Set("maxResults", strconv.Itoa(listConfig.MaxResults))
	}
	if listConfig.PageToken != "" {
		query.Set("pageToken", listConfig.PageToken)
	}
	if listConfig.TimeZone != "" {
		query.Set("timeZone", listConfig.TimeZone)
	}
	if listConfig.ShowDeleted {
		query.Set("showDeleted", "true")
	}

	return rest.GetJson[event_list.EventList](ctx, c.urlString(query, eventsPath(calendarId)...), c.fetchOptions(listConfig.FetchOptions))
}

// GetEvent retrieves an event.
func (c *Client) GetEvent(ctx context.Context, calendarId string, eventId string, options ...fetch_config.Option) (*event.Event, error) {
	if calendarId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("calendar id"))
	}
	if eventId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("event id"))
	}

	return rest.GetJson[event.Event](ctx, c.urlString(nil, eventPath(calendarId, eventId)...), c.fetchOptions(options))
}

// InsertEvent makes an event in the calendar, and returns it with its id.
func (c *Client) InsertEvent(
	ctx context.Context,
	calendarId string,
	e *event.Event,
	options ...write_event_config.Option,
) (*event.Event, error) {
	if calendarId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("calendar id"))
	}
	if e == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("event"))
	}
	if e.Start == nil || e.End == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("event start or end"))
	}

	writeConfig := write_event_config.New(options...)

	return rest.SendJson[event.Event](
		ctx,
		http.MethodPost,
		c.urlString(writeQuery(writeConfig), eventsPath(calendarId)...),
		e,
		c.fetchOptions(writeConfig.FetchOptions),
	)
}

// PatchEvent changes the fields of an event that the patch sets, leaving the rest; a list it sets replaces the event's
// whole. The patch is an *event.Event, which sets the fields it has a value in, or a map of the fields' JSON names to
// their values, which is how one is emptied: an event's empty field is not sent.
func (c *Client) PatchEvent(
	ctx context.Context,
	calendarId string,
	eventId string,
	patch any,
	options ...write_event_config.Option,
) (*event.Event, error) {
	if calendarId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("calendar id"))
	}
	if eventId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("event id"))
	}
	if utils.IsNil(patch) {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("patch"))
	}

	writeConfig := write_event_config.New(options...)

	return rest.SendJson[event.Event](
		ctx,
		http.MethodPatch,
		c.urlString(writeQuery(writeConfig), eventPath(calendarId, eventId)...),
		patch,
		c.fetchOptions(writeConfig.FetchOptions),
	)
}

// DeleteEvent deletes an event; it is kept as cancelled, which a listing shows only when asked to show the deleted.
func (c *Client) DeleteEvent(ctx context.Context, calendarId string, eventId string, options ...write_event_config.Option) error {
	if calendarId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("calendar id"))
	}
	if eventId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("event id"))
	}

	writeConfig := write_event_config.New(options...)

	return rest.Do(
		ctx,
		http.MethodDelete,
		c.urlString(writeQuery(writeConfig), eventPath(calendarId, eventId)...),
		c.fetchOptions(writeConfig.FetchOptions),
	)
}

// QueryFreeBusy retrieves when the calendars the request names are busy. A calendar the user may not see the busy
// times of comes back with an error of its own, and the call succeeds.
func (c *Client) QueryFreeBusy(ctx context.Context, request *free_busy.Request, options ...fetch_config.Option) (*free_busy.Response, error) {
	if request == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("free busy request"))
	}
	if len(request.Items) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("free busy items"))
	}

	return rest.SendJson[free_busy.Response](ctx, http.MethodPost, c.urlString(nil, "freeBusy"), request, c.fetchOptions(options))
}
