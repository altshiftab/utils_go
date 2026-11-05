package calendar

import (
	"context"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/calendar/calendar_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/calendar/list_events_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/calendar/types/event"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/calendar/types/free_busy"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/calendar/write_event_config"
)

func testServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return NewClient(calendar_config.WithBaseUrl(u))
}

// TestRequests checks that each call asks Calendar what it should -- method, path, query and body -- and reads back
// what Calendar answers.
func TestRequests(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 11, 5, 9, 0, 0, 0, time.FixedZone("CET", 3600))
	meeting := &event.Event{
		Summary:   "Möte",
		Start:     &event.DateTime{DateTime: "2026-11-05T09:00:00+01:00"},
		End:       &event.DateTime{DateTime: "2026-11-05T10:00:00+01:00"},
		Attendees: []*event.Attendee{{Email: "a@example.com"}},
	}

	testCases := []struct {
		name     string
		call     func(client *Client) (any, error)
		method   string
		path     string
		query    url.Values
		body     string
		response string
		// want is the result, as JSON; empty for a call that returns none.
		want string
	}{
		{
			name:     "list calendars",
			call:     func(client *Client) (any, error) { return client.ListCalendarsPage(context.Background(), "p2") },
			method:   http.MethodGet,
			path:     "/calendar/v3/users/me/calendarList",
			query:    url.Values{"pageToken": {"p2"}},
			response: `{"items":[{"id":"a@example.com","summary":"A","primary":true,"accessRole":"owner"}]}`,
			want:     `{"items":[{"id":"a@example.com","summary":"A","accessRole":"owner","primary":true}]}`,
		},
		{
			name: "list events",
			call: func(client *Client) (any, error) {
				return client.ListEventsPage(
					context.Background(),
					"family@group.calendar.google.com",
					list_events_config.WithTimeMin(start),
					list_events_config.WithTimeMax(start.Add(24*time.Hour)),
					list_events_config.WithQuery("tandläkare"),
					list_events_config.WithSingleEvents(true),
					list_events_config.WithOrderBy(list_events_config.OrderByStartTime),
					list_events_config.WithMaxResults(10),
					list_events_config.WithPageToken("next"),
					list_events_config.WithTimeZone("Europe/Stockholm"),
				)
			},
			method: http.MethodGet,
			path:   "/calendar/v3/calendars/family@group.calendar.google.com/events",
			query: url.Values{
				"timeMin": {"2026-11-05T09:00:00+01:00"}, "timeMax": {"2026-11-06T09:00:00+01:00"}, "q": {"tandläkare"},
				"singleEvents": {"true"}, "orderBy": {"startTime"}, "maxResults": {"10"}, "pageToken": {"next"},
				"timeZone": {"Europe/Stockholm"},
			},
			response: `{"items":[{"id":"e1","summary":"Tandläkare","start":{"dateTime":"2026-11-05T14:00:00+01:00"}}],"timeZone":"Europe/Stockholm"}`,
			want:     `{"items":[{"id":"e1","summary":"Tandläkare","start":{"dateTime":"2026-11-05T14:00:00+01:00"}}],"timeZone":"Europe/Stockholm"}`,
		},
		{
			name:     "get event",
			call:     func(client *Client) (any, error) { return client.GetEvent(context.Background(), Primary, "e1") },
			method:   http.MethodGet,
			path:     "/calendar/v3/calendars/primary/events/e1",
			response: `{"id":"e1","attendees":[{"email":"a@example.com","responseStatus":"needsAction","self":true}]}`,
			want:     `{"id":"e1","attendees":[{"email":"a@example.com","responseStatus":"needsAction","self":true}]}`,
		},
		{
			name: "insert event, with a meeting asked for",
			call: func(client *Client) (any, error) {
				return client.InsertEvent(
					context.Background(), Primary, meeting,
					write_event_config.WithSendUpdates(write_event_config.SendUpdatesAll),
					write_event_config.WithConferenceDataVersion(1),
				)
			},
			method: http.MethodPost,
			path:   "/calendar/v3/calendars/primary/events",
			query:  url.Values{"sendUpdates": {"all"}, "conferenceDataVersion": {"1"}},
			body: `{"summary":"Möte","start":{"dateTime":"2026-11-05T09:00:00+01:00"},"end":{"dateTime":"2026-11-05T10:00:00+01:00"},` +
				`"attendees":[{"email":"a@example.com"}]}`,
			response: `{"id":"new","htmlLink":"https://calendar.google.com/x"}`,
			want:     `{"id":"new","htmlLink":"https://calendar.google.com/x"}`,
		},
		{
			name: "patch event",
			call: func(client *Client) (any, error) {
				return client.PatchEvent(context.Background(), Primary, "e1", &event.Event{Location: "Rum 3"})
			},
			method:   http.MethodPatch,
			path:     "/calendar/v3/calendars/primary/events/e1",
			body:     `{"location":"Rum 3"}`,
			response: `{"id":"e1","location":"Rum 3"}`,
			want:     `{"id":"e1","location":"Rum 3"}`,
		},
		{
			name: "patch event by fields, one emptied",
			call: func(client *Client) (any, error) {
				return client.PatchEvent(context.Background(), Primary, "e1", map[string]any{"location": ""})
			},
			method:   http.MethodPatch,
			path:     "/calendar/v3/calendars/primary/events/e1",
			body:     `{"location":""}`,
			response: `{"id":"e1"}`,
			want:     `{"id":"e1"}`,
		},
		{
			name: "delete event",
			call: func(client *Client) (any, error) {
				return nil, client.DeleteEvent(context.Background(), Primary, "e1", write_event_config.WithSendUpdates(write_event_config.SendUpdatesNone))
			},
			method: http.MethodDelete,
			path:   "/calendar/v3/calendars/primary/events/e1",
			query:  url.Values{"sendUpdates": {"none"}},
		},
		{
			name: "free busy",
			call: func(client *Client) (any, error) {
				return client.QueryFreeBusy(context.Background(), &free_busy.Request{
					TimeMin: "2026-11-05T08:00:00Z", TimeMax: "2026-11-05T17:00:00Z", Items: []*free_busy.Item{{Id: "a@example.com"}},
				})
			},
			method:   http.MethodPost,
			path:     "/calendar/v3/freeBusy",
			body:     `{"timeMin":"2026-11-05T08:00:00Z","timeMax":"2026-11-05T17:00:00Z","items":[{"id":"a@example.com"}]}`,
			response: `{"calendars":{"a@example.com":{"busy":[{"start":"2026-11-05T09:00:00Z","end":"2026-11-05T10:00:00Z"}]}}}`,
			want:     `{"calendars":{"a@example.com":{"busy":[{"start":"2026-11-05T09:00:00Z","end":"2026-11-05T10:00:00Z"}]}}}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != testCase.method {
					t.Errorf("method = %s, want %s", r.Method, testCase.method)
				}
				if r.URL.Path != testCase.path {
					t.Errorf("path = %s, want %s", r.URL.Path, testCase.path)
				}
				query := r.URL.Query()
				if len(query) != len(testCase.query) {
					t.Errorf("query = %v, want %v", query, testCase.query)
				}
				for name, want := range testCase.query {
					if got := query[name]; strings.Join(got, ",") != strings.Join(want, ",") {
						t.Errorf("query %s = %v, want %v", name, got, want)
					}
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read body: %v", err)
				}
				if testCase.body != "" && strings.TrimSpace(string(body)) != testCase.body {
					t.Errorf("body = %s, want %s", body, testCase.body)
				}
				if testCase.response == "" {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, testCase.response)
			})

			result, err := testCase.call(client)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if testCase.want == "" {
				return
			}
			got, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("marshal result: %v", err)
			}
			if string(got) != testCase.want {
				t.Errorf("result = %s, want %s", got, testCase.want)
			}
		})
	}
}

// TestArgumentErrors checks that a call missing what names its target is refused before anything is asked of
// Calendar.
func TestArgumentErrors(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})
	ctx := context.Background()
	timed := &event.Event{Start: &event.DateTime{Date: "2026-11-05"}, End: &event.DateTime{Date: "2026-11-06"}}

	testCases := []struct {
		name string
		call func() error
	}{
		{name: "list events, no calendar", call: func() error { _, err := client.ListEventsPage(ctx, ""); return err }},
		{name: "get event, no calendar", call: func() error { _, err := client.GetEvent(ctx, "", "e1"); return err }},
		{name: "get event, no event", call: func() error { _, err := client.GetEvent(ctx, Primary, ""); return err }},
		{name: "insert, no calendar", call: func() error { _, err := client.InsertEvent(ctx, "", timed); return err }},
		{name: "insert, nil event", call: func() error { _, err := client.InsertEvent(ctx, Primary, nil); return err }},
		{name: "insert, no end", call: func() error { _, err := client.InsertEvent(ctx, Primary, &event.Event{Start: timed.Start}); return err }},
		{name: "patch, no event id", call: func() error { _, err := client.PatchEvent(ctx, Primary, "", timed); return err }},
		{name: "patch, nil", call: func() error { _, err := client.PatchEvent(ctx, Primary, "e1", nil); return err }},
		{name: "patch, nil event", call: func() error { _, err := client.PatchEvent(ctx, Primary, "e1", (*event.Event)(nil)); return err }},
		{name: "delete, no event", call: func() error { return client.DeleteEvent(ctx, Primary, "") }},
		{name: "free busy, nil", call: func() error { _, err := client.QueryFreeBusy(ctx, nil); return err }},
		{name: "free busy, no items", call: func() error { _, err := client.QueryFreeBusy(ctx, &free_busy.Request{}); return err }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if err := testCase.call(); err == nil {
				t.Errorf("%s: expected an error", testCase.name)
			}
		})
	}
}
