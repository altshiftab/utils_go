package rest

import (
	"net/url"
	"testing"
)

func TestUrl(t *testing.T) {
	t.Parallel()

	base := &url.URL{Scheme: "https", Host: "www.googleapis.com", Path: "/calendar/v3/"}

	testCases := []struct {
		name string
		// base is the calendar base when nil.
		base     *url.URL
		query    url.Values
		segments []string
		want     string
		// wantPath is the path as a server reads it, decoded.
		wantPath string
	}{
		{
			name:     "plain segments",
			segments: []string{"users", "me", "calendarList"},
			want:     "https://www.googleapis.com/calendar/v3/users/me/calendarList",
			wantPath: "/calendar/v3/users/me/calendarList",
		},
		{
			name:     "no segments",
			want:     "https://www.googleapis.com/calendar/v3/",
			wantPath: "/calendar/v3/",
		},
		{
			name:     "hash escaped once",
			segments: []string{"calendars", "sv.swedish#holiday@group.v.calendar.google.com", "events"},
			want:     "https://www.googleapis.com/calendar/v3/calendars/sv.swedish%23holiday@group.v.calendar.google.com/events",
			wantPath: "/calendar/v3/calendars/sv.swedish#holiday@group.v.calendar.google.com/events",
		},
		{
			name:     "slash kept within its segment",
			segments: []string{"files", "a/b"},
			want:     "https://www.googleapis.com/calendar/v3/files/a%2Fb",
			wantPath: "/calendar/v3/files/a/b",
		},
		{
			// URL.String would leave ';' and ',' bare in a path, so this passes only if RawPath is used.
			name:     "semicolon and comma",
			segments: []string{"a;b,c"},
			want:     "https://www.googleapis.com/calendar/v3/a%3Bb%2Cc",
			wantPath: "/calendar/v3/a;b,c",
		},
		{
			name:     "empty segment",
			segments: []string{"a", "", "b"},
			want:     "https://www.googleapis.com/calendar/v3/a//b",
			wantPath: "/calendar/v3/a//b",
		},
		{
			name:     "base without a trailing slash",
			base:     &url.URL{Scheme: "https", Host: "www.googleapis.com", Path: "/v1"},
			segments: []string{"users"},
			want:     "https://www.googleapis.com/v1/users",
			wantPath: "/v1/users",
		},
		{
			name:     "base with an escaped path",
			base:     &url.URL{Scheme: "https", Host: "www.googleapis.com", Path: "/pre/fix/", RawPath: "/pre%2Ffix/"},
			segments: []string{"x#y"},
			want:     "https://www.googleapis.com/pre%2Ffix/x%23y",
			wantPath: "/pre/fix/x#y",
		},
		{
			name:     "percent, space and question mark",
			segments: []string{"100% sure?"},
			want:     "https://www.googleapis.com/calendar/v3/100%25%20sure%3F",
			wantPath: "/calendar/v3/100% sure?",
		},
		{
			name:     "non-ASCII",
			segments: []string{"Helgdagar i Sverige ö"},
			want:     "https://www.googleapis.com/calendar/v3/Helgdagar%20i%20Sverige%20%C3%B6",
			wantPath: "/calendar/v3/Helgdagar i Sverige ö",
		},
		{
			name:     "query",
			query:    url.Values{"alt": {"json"}, "q": {"a b"}},
			segments: []string{"groups", "team@example.com"},
			want:     "https://www.googleapis.com/calendar/v3/groups/team@example.com?alt=json&q=a+b",
			wantPath: "/calendar/v3/groups/team@example.com",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			testBase := testCase.base
			if testBase == nil {
				testBase = base
			}

			got := Url(testBase, testCase.query, testCase.segments...)
			if got != testCase.want {
				t.Errorf("Url = %s, want %s", got, testCase.want)
			}

			parsed, err := url.Parse(got)
			if err != nil {
				t.Fatalf("parse %s: %v", got, err)
			}
			if parsed.Path != testCase.wantPath {
				t.Errorf("decoded path = %s, want %s", parsed.Path, testCase.wantPath)
			}
		})
	}
}
