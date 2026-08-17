package generate_content_response

import (
	"encoding/json/v2"
	"reflect"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/candidate"
	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/content"
	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/part"
)

func TestText(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		response *GenerateContentResponse
		want     string
	}{
		{
			name:     "nil response",
			response: nil,
			want:     "",
		},
		{
			name:     "no candidates",
			response: &GenerateContentResponse{},
			want:     "",
		},
		{
			name:     "nil first candidate",
			response: &GenerateContentResponse{Candidates: []*candidate.Candidate{nil}},
			want:     "",
		},
		{
			name:     "nil content",
			response: &GenerateContentResponse{Candidates: []*candidate.Candidate{{}}},
			want:     "",
		},
		{
			name: "concatenates non-thought parts and skips nil",
			response: &GenerateContentResponse{
				Candidates: []*candidate.Candidate{
					{Content: &content.Content{Parts: []*part.Part{
						{Text: "Hello, "},
						{Text: "internal reasoning", Thought: true},
						{Text: "world"},
						nil,
						{Text: "!"},
					}}},
				},
			},
			want: "Hello, world!",
		},
		{
			name: "uses only the first candidate",
			response: &GenerateContentResponse{
				Candidates: []*candidate.Candidate{
					{Content: &content.Content{Parts: []*part.Part{{Text: "first"}}}},
					{Content: &content.Content{Parts: []*part.Part{{Text: "second"}}}},
				},
			},
			want: "first",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := testCase.response.Text(); got != testCase.want {
				t.Errorf("Text() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestGenerateContentResponseJSONRoundTrip(t *testing.T) {
	t.Parallel()

	input := `{` +
		`"candidates":[{"content":{"role":"model","parts":[{"text":"hi"}]},"finishReason":"STOP","index":0}],` +
		`"promptFeedback":{"blockReason":"SAFETY"},` +
		`"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":3,"thoughtsTokenCount":1,"totalTokenCount":9},` +
		`"modelVersion":"gemini-2.0-flash"}`

	var response GenerateContentResponse
	if err := json.Unmarshal([]byte(input), &response); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if response.ModelVersion != "gemini-2.0-flash" {
		t.Errorf("ModelVersion = %q, want %q", response.ModelVersion, "gemini-2.0-flash")
	}
	if response.Text() != "hi" {
		t.Errorf("Text() = %q, want %q", response.Text(), "hi")
	}
	if len(response.Candidates) != 1 {
		t.Fatalf("Candidates len = %d, want 1", len(response.Candidates))
	}
	if response.Candidates[0].FinishReason != "STOP" {
		t.Errorf("FinishReason = %q, want %q", response.Candidates[0].FinishReason, "STOP")
	}
	if response.PromptFeedback == nil || response.PromptFeedback.BlockReason != "SAFETY" {
		t.Errorf("PromptFeedback = %#v, want BlockReason SAFETY", response.PromptFeedback)
	}
	if response.UsageMetadata == nil || response.UsageMetadata.TotalTokenCount != 9 {
		t.Errorf("UsageMetadata = %#v, want TotalTokenCount 9", response.UsageMetadata)
	}

	data, err := json.Marshal(&response)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var reparsed GenerateContentResponse
	if err := json.Unmarshal(data, &reparsed); err != nil {
		t.Fatalf("Unmarshal (reparse) error: %v", err)
	}
	if !reflect.DeepEqual(response, reparsed) {
		t.Errorf("round trip mismatch:\n got  %#v\n want %#v", reparsed, response)
	}
}
