// Package llm provides a provider-agnostic interface for LLM-powered financial tips.
//
// NudgeLLM interface: Generate(ctx, prompt) (string, error).
// Four adapters: openai-compatible (POST /v1/chat/completions), openai (POST /v1/responses),
// anthropic (POST /v1/messages), disabled (no-op).
//
// All adapters are shell-layer only — no business logic, no import of internal/senpai.
// No external SDK dependencies — pure net/http calls.
package llm

import (
	"fmt"
	"os"
)

// Provider identifies the LLM backend.
type Provider string

const (
	// ProviderOpenAICompatible targets any OpenAI-compatible API (Ollama, Groq, Gemini,
	// Together, etc.) via POST /v1/chat/completions.
	ProviderOpenAICompatible Provider = "openai-compatible"

	// ProviderOpenAI targets the newer OpenAI Responses API via POST /v1/responses.
	ProviderOpenAI Provider = "openai"

	// ProviderAnthropic targets the Anthropic Messages API via POST /v1/messages.
	ProviderAnthropic Provider = "anthropic"

	// ProviderDisabled is the no-op adapter — returns empty string without any HTTP call.
	ProviderDisabled Provider = "disabled"
)

// LLMConfig holds the configuration for the LLM provider.
// All fields are populated from environment variables.
type LLMConfig struct {
	// Provider is the LLM backend to use ("openai-compatible", "openai", "anthropic", "disabled").
	Provider Provider

	// BaseURL is the base URL for the API (e.g., "https://api.openai.com").
	// For openai-compatible, this may point to a local server like "http://localhost:11434".
	// Defaults to provider-specific defaults when empty.
	BaseURL string

	// APIKey is the API key for authentication.
	APIKey string

	// Model is the model identifier (e.g., "gpt-4o", "claude-3-opus-20240229", "llama3").
	Model string
}

// defaultBaseURLs maps provider to their default base URLs.
var defaultBaseURLs = map[Provider]string{
	ProviderOpenAICompatible: "",       // must be explicitly set
	ProviderOpenAI:           "https://api.openai.com",
	ProviderAnthropic:        "https://api.anthropic.com",
	ProviderDisabled:         "",
}

// ParseProvider parses a provider string into a Provider type.
// Returns ProviderDisabled for unknown or empty values.
func ParseProvider(s string) Provider {
	switch s {
	case string(ProviderOpenAICompatible):
		return ProviderOpenAICompatible
	case string(ProviderOpenAI):
		return ProviderOpenAI
	case string(ProviderAnthropic):
		return ProviderAnthropic
	case string(ProviderDisabled), "":
		return ProviderDisabled
	default:
		return ProviderDisabled
	}
}

// EnvVar names for LLM configuration.
const (
	EnvProvider = "SENPAI_LLM_PROVIDER"
	EnvBaseURL  = "SENPAI_LLM_BASE_URL"
	EnvAPIKey   = "SENPAI_LLM_API_KEY"
	EnvModel    = "SENPAI_LLM_MODEL"
)

// LoadConfigFromEnv reads LLM configuration from environment variables.
//
// Env vars:
//   - SENPAI_LLM_PROVIDER  (default: "disabled")
//   - SENPAI_LLM_BASE_URL   (default: provider-specific)
//   - SENPAI_LLM_API_KEY    (default: "")
//   - SENPAI_LLM_MODEL      (default: "")
func LoadConfigFromEnv() LLMConfig {
	provider := ParseProvider(os.Getenv(EnvProvider))
	baseURL := os.Getenv(EnvBaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURLs[provider]
	}

	return LLMConfig{
		Provider: provider,
		BaseURL:  baseURL,
		APIKey:   os.Getenv(EnvAPIKey),
		Model:    os.Getenv(EnvModel),
	}
}

// Validate checks that the configuration is usable for the given provider.
// Returns nil for disabled or empty provider. For others, ensures BaseURL, APIKey,
// and Model are all non-empty.
func (c LLMConfig) Validate() error {
	if c.Provider == ProviderDisabled || c.Provider == "" {
		return nil
	}
	if c.BaseURL == "" {
		return fmt.Errorf("llm: %s requires a non-empty base URL", c.Provider)
	}
	if c.APIKey == "" {
		return fmt.Errorf("llm: %s requires a non-empty API key", c.Provider)
	}
	if c.Model == "" {
		return fmt.Errorf("llm: %s requires a non-empty model", c.Provider)
	}
	return nil
}

// IsEnabled returns true when the provider is not disabled and not empty.
func (c LLMConfig) IsEnabled() bool {
	return c.Provider != ProviderDisabled && c.Provider != ""
}
