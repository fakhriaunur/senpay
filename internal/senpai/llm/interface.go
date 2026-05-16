package llm

import "context"

// NudgeLLM is the provider-agnostic interface for generating financial nudge tips.
//
// Implementations must be safe for concurrent use.
// When the provider is disabled or an error occurs, implementations return ("", nil).
type NudgeLLM interface {
	// Generate sends a prompt to the LLM and returns the generated text response.
	// Returns ("", nil) when the adapter is disabled.
	// Returns an error on network failures, non-200 responses, or JSON parse errors.
	Generate(ctx context.Context, prompt string) (string, error)
}
