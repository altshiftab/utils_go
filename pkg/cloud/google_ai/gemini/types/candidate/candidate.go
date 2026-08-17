package candidate

import (
	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/content"
)

type Candidate struct {
	Content      *content.Content `json:"content,omitempty"`
	FinishReason string           `json:"finishReason,omitempty"`
	Index        int              `json:"index,omitzero"`
}
