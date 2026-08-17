package content

import (
	"encoding/json/v2"
	"reflect"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/part"
)

func TestNewText(t *testing.T) {
	t.Parallel()

	c := NewText("user", "hello")
	if c == nil {
		t.Fatal("NewText returned nil")
	}
	if c.Role != "user" {
		t.Errorf("Role = %q, want %q", c.Role, "user")
	}
	if len(c.Parts) != 1 {
		t.Fatalf("Parts len = %d, want 1", len(c.Parts))
	}
	if c.Parts[0] == nil {
		t.Fatal("Parts[0] is nil")
	}
	if c.Parts[0].Text != "hello" {
		t.Errorf("Parts[0].Text = %q, want %q", c.Parts[0].Text, "hello")
	}
	if c.Parts[0].Thought {
		t.Error("Parts[0].Thought = true, want false")
	}
}

func TestContentJSONRoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		content  *Content
		wantJSON string
	}{
		{
			name:     "role and text",
			content:  &Content{Role: "model", Parts: []*part.Part{{Text: "hi"}}},
			wantJSON: `{"role":"model","parts":[{"text":"hi"}]}`,
		},
		{
			name:     "role omitted when empty",
			content:  &Content{Parts: []*part.Part{{Text: "system instruction"}}},
			wantJSON: `{"parts":[{"text":"system instruction"}]}`,
		},
		{
			name:     "thought part",
			content:  &Content{Role: "model", Parts: []*part.Part{{Text: "reasoning", Thought: true}}},
			wantJSON: `{"role":"model","parts":[{"text":"reasoning","thought":true}]}`,
		},
		{
			name:     "empty",
			content:  &Content{},
			wantJSON: `{}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(testCase.content)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			if string(data) != testCase.wantJSON {
				t.Errorf("Marshal = %s, want %s", data, testCase.wantJSON)
			}

			var got Content
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if !reflect.DeepEqual(&got, testCase.content) {
				t.Errorf("round trip = %#v, want %#v", &got, testCase.content)
			}
		})
	}
}
