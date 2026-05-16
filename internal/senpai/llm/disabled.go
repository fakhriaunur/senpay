package llm

import "context"

// disabledAdapter is a no-op adapter that returns an empty string without
// making any HTTP calls. Used when the LLM feature is disabled.
type disabledAdapter struct{}

// Generate returns ("", nil) — no-op. No HTTP call is made.
func (d *disabledAdapter) Generate(_ context.Context, _ string) (string, error) {
	return "", nil
}
