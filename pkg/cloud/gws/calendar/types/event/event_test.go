package event

import (
	"encoding/json/v2"
	"testing"
)

func TestEventJson(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		event *Event
		want  string
	}{
		{
			name:  "no reminders at all says so",
			event: &Event{Summary: "a", Reminders: &Reminders{}},
			want:  `{"summary":"a","reminders":{"useDefault":false}}`,
		},
		{
			name:  "a reminder at the start",
			event: &Event{Reminders: &Reminders{Overrides: []*Reminder{{Method: "popup", Minutes: 0}, {Method: "email", Minutes: 60}}}},
			want:  `{"reminders":{"useDefault":false,"overrides":[{"method":"popup","minutes":0},{"method":"email","minutes":60}]}}`,
		},
		{
			name: "out of office",
			event: &Event{
				EventType:             "outOfOffice",
				OutOfOfficeProperties: &OutOfOfficeProperties{AutoDeclineMode: AutoDeclineAll, DeclineMessage: "Away"},
			},
			want: `{"eventType":"outOfOffice","outOfOfficeProperties":{"autoDeclineMode":"declineAllConflictingInvitations","declineMessage":"Away"}}`,
		},
		{
			name:  "focus time",
			event: &Event{EventType: "focusTime", FocusTimeProperties: &FocusTimeProperties{AutoDeclineMode: AutoDeclineNew, ChatStatus: "doNotDisturb"}},
			want:  `{"eventType":"focusTime","focusTimeProperties":{"autoDeclineMode":"declineOnlyNewConflictingInvitations","chatStatus":"doNotDisturb"}}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := json.Marshal(testCase.event)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(got) != testCase.want {
				t.Errorf("json = %s, want %s", got, testCase.want)
			}
		})
	}
}
