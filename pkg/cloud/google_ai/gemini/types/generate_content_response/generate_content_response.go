package generate_content_response

import (
	"strings"

	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/candidate"
	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/prompt_feedback"
	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/usage_metadata"
)

type GenerateContentResponse struct {
	Candidates     []*candidate.Candidate          `json:"candidates,omitempty"`
	PromptFeedback *prompt_feedback.PromptFeedback `json:"promptFeedback,omitempty"`
	UsageMetadata  *usage_metadata.UsageMetadata   `json:"usageMetadata,omitempty"`
	ModelVersion   string                          `json:"modelVersion,omitempty"`
}

// Text returns the concatenated text of the first candidate, skipping thought parts.
func (response *GenerateContentResponse) Text() string {
	if response == nil || len(response.Candidates) == 0 {
		return ""
	}

	firstCandidate := response.Candidates[0]
	if firstCandidate == nil || firstCandidate.Content == nil {
		return ""
	}

	var builder strings.Builder
	for _, contentPart := range firstCandidate.Content.Parts {
		if contentPart == nil || contentPart.Thought {
			continue
		}
		builder.WriteString(contentPart.Text)
	}

	return builder.String()
}
