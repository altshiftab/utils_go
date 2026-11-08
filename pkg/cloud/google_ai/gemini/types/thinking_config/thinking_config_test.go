package thinking_config

import (
	"encoding/json/v2"
	"testing"
)

func TestThinkingConfigJSON(t *testing.T) {
	t.Parallel()

	budget := 1024

	testCases := []struct {
		name   string
		config *ThinkingConfig
		want   string
	}{
		{name: "level", config: &ThinkingConfig{ThinkingLevel: "high"}, want: `{"thinkingLevel":"high"}`},
		{name: "budget", config: &ThinkingConfig{ThinkingBudget: &budget}, want: `{"thinkingBudget":1024}`},
		{name: "thoughts", config: &ThinkingConfig{IncludeThoughts: true}, want: `{"includeThoughts":true}`},
		{name: "empty", config: &ThinkingConfig{}, want: `{}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			raw, err := json.Marshal(testCase.config)
			if err != nil {
				t.Fatalf("json marshal: %v", err)
			}

			if string(raw) != testCase.want {
				t.Errorf("got %s, want %s", raw, testCase.want)
			}
		})
	}
}
