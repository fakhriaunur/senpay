package llm

import (
	"context"
	"testing"
)

func TestAdapter_NewNudgeLLM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  LLMConfig
		want string
	}{
		{"disabled_provider", LLMConfig{Provider: ProviderDisabled}, "*llm.disabledAdapter"},
		{"empty_provider", LLMConfig{Provider: ""}, "*llm.disabledAdapter"},
		{"invalid_provider", LLMConfig{Provider: "invalid"}, "*llm.disabledAdapter"},
		{"openai_compatible_provider", LLMConfig{Provider: ProviderOpenAICompatible}, "*llm.openAICompatibleAdapter"},
		{"openai_provider", LLMConfig{Provider: ProviderOpenAI}, "*llm.openAIAdapter"},
		{"anthropic_provider", LLMConfig{Provider: ProviderAnthropic}, "*llm.anthropicAdapter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewNudgeLLM(tt.cfg)
			got := adapterTypeName(adapter)
			if got != tt.want {
				t.Errorf("NewNudgeLLM(%q) returned %s, want %s", tt.cfg.Provider, got, tt.want)
			}
		})
	}
}

func adapterTypeName(v interface{}) string {
	switch v.(type) {
	case *disabledAdapter:
		return "*llm.disabledAdapter"
	case *openAICompatibleAdapter:
		return "*llm.openAICompatibleAdapter"
	case *openAIAdapter:
		return "*llm.openAIAdapter"
	case *anthropicAdapter:
		return "*llm.anthropicAdapter"
	default:
		return "*llm.unknown"
	}
}

func TestDisabledAdapter_Generate(t *testing.T) {
	t.Parallel()

	adapter := &disabledAdapter{}
	result, err := adapter.Generate(context.Background(), "some prompt")
	if err != nil {
		t.Errorf("disabledAdapter.Generate() returned error: %v", err)
	}
	if result != "" {
		t.Errorf("disabledAdapter.Generate() = %q, want empty string", result)
	}
}

func TestParseProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  Provider
	}{
		{"openai-compatible", ProviderOpenAICompatible},
		{"openai", ProviderOpenAI},
		{"anthropic", ProviderAnthropic},
		{"disabled", ProviderDisabled},
		{"", ProviderDisabled},
		{"unknown-value", ProviderDisabled},
		{"OPENAI-COMPATIBLE", ProviderDisabled},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseProvider(tt.input)
			if got != tt.want {
				t.Errorf("ParseProvider(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLLMConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     LLMConfig
		wantErr bool
	}{
		{"disabled_valid", LLMConfig{Provider: ProviderDisabled}, false},
		{"empty_provider_valid_as_disabled", LLMConfig{}, false},
		{"openai_no_baseurl", LLMConfig{Provider: ProviderOpenAI, APIKey: "key", Model: "gpt-4o"}, true},
		{"openai_no_apikey", LLMConfig{Provider: ProviderOpenAI, BaseURL: "https://api.openai.com", Model: "gpt-4o"}, true},
		{"openai_no_model", LLMConfig{Provider: ProviderOpenAI, BaseURL: "https://api.openai.com", APIKey: "key"}, true},
		{"openai_valid", LLMConfig{Provider: ProviderOpenAI, BaseURL: "https://api.openai.com", APIKey: "key", Model: "gpt-4o"}, false},
		{"anthropic_valid", LLMConfig{Provider: ProviderAnthropic, BaseURL: "https://api.anthropic.com", APIKey: "key", Model: "claude-3"}, false},
		{"openai_compatible_valid", LLMConfig{Provider: ProviderOpenAICompatible, BaseURL: "http://localhost:11434", APIKey: "ollama", Model: "llama3"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestLLMConfig_IsEnabled(t *testing.T) {
	t.Parallel()

	cfgDisabled := LLMConfig{Provider: ProviderDisabled}
	if cfgDisabled.IsEnabled() {
		t.Error("disabled provider should not be enabled")
	}

	cfgEmpty := LLMConfig{}
	if cfgEmpty.IsEnabled() {
		t.Error("empty provider should not be enabled")
	}

	cfgOpenAI := LLMConfig{Provider: ProviderOpenAI}
	if !cfgOpenAI.IsEnabled() {
		t.Error("openai provider should be enabled")
	}

	cfgAnthropic := LLMConfig{Provider: ProviderAnthropic}
	if !cfgAnthropic.IsEnabled() {
		t.Error("anthropic provider should be enabled")
	}

	cfgCompat := LLMConfig{Provider: ProviderOpenAICompatible}
	if !cfgCompat.IsEnabled() {
		t.Error("openai-compatible provider should be enabled")
	}
}
