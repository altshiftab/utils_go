package generate_content_request

import (
	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/content"
	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/generation_config"
)

type GenerateContentRequest struct {
	Contents          []*content.Content                  `json:"contents"`
	SystemInstruction *content.Content                    `json:"systemInstruction,omitempty"`
	GenerationConfig  *generation_config.GenerationConfig `json:"generationConfig,omitempty"`
	// Store says whether Google keeps the request and its answer in the project's logs, overriding the
	// project's setting; nil leaves the setting in force, which keeps them in projects created since
	// 2026-10-26.
	Store *bool `json:"store,omitzero"`
}
