package force_send

import (
	"encoding/json/v2"
	"testing"
)

type sample struct {
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	Hidden      string   `json:"-"`
}

func TestMarshal(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		value  any
		fields []string
		want   map[string]any
	}{
		{name: "nothing forced", value: &sample{Name: "a"}, want: map[string]any{"name": "a"}},
		{name: "empty string forced", value: &sample{Name: "a"}, fields: []string{"description"}, want: map[string]any{"name": "a", "description": ""}},
		{name: "empty list forced", value: sample{}, fields: []string{"aliases"}, want: map[string]any{"aliases": nil}},
		{name: "set value left as it is", value: &sample{Description: "d"}, fields: []string{"description"}, want: map[string]any{"description": "d"}},
		{name: "unknown and hidden names ignored", value: &sample{}, fields: []string{"nope", "-", "Hidden"}, want: map[string]any{}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			encoded, err := Marshal(testCase.value, testCase.fields)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatalf("unmarshal %s: %v", encoded, err)
			}
			if len(got) != len(testCase.want) {
				t.Fatalf("got %s, want %v", encoded, testCase.want)
			}
			for key, want := range testCase.want {
				value, present := got[key]
				if !present {
					t.Errorf("%s: %q missing", encoded, key)
					continue
				}
				if want != nil && value != want {
					t.Errorf("%s: %q = %v, want %v", encoded, key, value, want)
				}
			}
		})
	}
}
