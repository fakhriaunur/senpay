package llm

// NewNudgeLLM creates the appropriate NudgeLLM adapter based on the provider
// specified in the configuration.
//
// Returns:
//   - disabled adapter when Provider is "disabled" or empty
//   - openai-compatible adapter when Provider is "openai-compatible"
//   - openai adapter when Provider is "openai"
//   - anthropic adapter when Provider is "anthropic"
func NewNudgeLLM(cfg LLMConfig) NudgeLLM {
	switch {
	case cfg.Provider == ProviderOpenAICompatible:
		return &openAICompatibleAdapter{config: cfg}
	case cfg.Provider == ProviderOpenAI:
		return &openAIAdapter{config: cfg}
	case cfg.Provider == ProviderAnthropic:
		return &anthropicAdapter{config: cfg}
	default:
		// Handles ProviderDisabled, empty string, and any unknown value
		return &disabledAdapter{}
	}
}
