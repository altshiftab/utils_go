package group

import (
	"encoding/json/v2"
	"strings"
	"testing"
)

func TestGroup_ForceSendFields(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		group       *Group
		wantPresent bool
	}{
		{name: "empty description left out", group: &Group{Name: "team"}},
		{name: "empty description forced", group: &Group{Name: "team", ForceSendFields: []string{"description"}}, wantPresent: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			encoded, err := json.Marshal(testCase.group)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if present := strings.Contains(string(encoded), `"description":""`); present != testCase.wantPresent {
				t.Errorf("%s: description sent %v, want %v", encoded, present, testCase.wantPresent)
			}
			if strings.Contains(string(encoded), "ForceSendFields") {
				t.Errorf("%s: the list of fields was sent", encoded)
			}

			var decoded Group
			if err := json.Unmarshal(encoded, &decoded); err != nil || decoded.Name != "team" {
				t.Errorf("decode %s: %+v, %v", encoded, decoded, err)
			}
		})
	}
}
