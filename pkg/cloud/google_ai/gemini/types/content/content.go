package content

import (
	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/part"
)

type Content struct {
	Role  string       `json:"role,omitempty"`
	Parts []*part.Part `json:"parts,omitempty"`
}

// NewText makes a single-part text content with the given role. The Gemini API
// uses the roles "user" and "model"; system instructions omit the role.
func NewText(role string, text string) *Content {
	return &Content{Role: role, Parts: []*part.Part{{Text: text}}}
}
