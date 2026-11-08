package thinking_config

type ThinkingConfig struct {
	ThinkingBudget  *int `json:"thinkingBudget,omitempty"`
	IncludeThoughts bool `json:"includeThoughts,omitzero"`
	// ThinkingLevel is how much the model thinks: "low", "medium" or "high" on Gemini 4, which ignores a
	// budget. It cannot be sent together with ThinkingBudget.
	ThinkingLevel string `json:"thinkingLevel,omitzero"`
}
