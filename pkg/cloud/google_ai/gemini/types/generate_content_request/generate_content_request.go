package generate_content_request

import (
	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/content"
	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/generation_config"
)

type GenerateContentRequest struct {
	Contents          []*content.Content                  `json:"contents"`
	SystemInstruction *content.Content                    `json:"systemInstruction,omitempty"`
	GenerationConfig  *generation_config.GenerationConfig `json:"generationConfig,omitempty"`
}
