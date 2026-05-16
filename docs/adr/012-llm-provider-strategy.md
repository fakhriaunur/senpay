# ADR 012: LLM Provider Strategy — Provider-Agnostic Adapter, No SDKs

**Status:** Accepted  
**Date:** 2026-05-16  
**Deciders:** Senpay Engineering Team

## Context

ADR 011 introduced an optional LLM tip-enhancement layer for the Senpai nudge engine. We need a strategy for integrating with LLM providers that:

- Avoids vendor lock-in — must support multiple providers (OpenAI, Anthropic, OpenAI-compatible)
- Minimizes dependency footprint — no third-party SDKs dragging in transitive dependencies
- Handles LLM failure gracefully — nudge engine must function without LLM
- Is provider-agnostic — adding a new provider should require minimal code
- Respects the FCIS pattern — LLM calls are I/O and belong in the shell

## Decision

Use a **single `NudgeLLM` interface** with **four thin adapters**, each implemented with raw `net/http` and typed request/response structs. No third-party SDKs. Provider selection via `LLM_PROVIDER` environment variable.

### Interface

```go
// NudgeLLM generates a tip for a nudge. Returns empty string on failure
// or when disabled — callers must handle gracefully.
type NudgeLLM interface {
    Generate(ctx context.Context, prompt string) (string, error)
}
```

Single method, single responsibility. Callers get a tip string or an error. No streaming, no chat history management, no token counting — those are unnecessary for nudge tip generation.

### Adapter Implementations

| Adapter | Provider(s) | Endpoint | Auth | When Selected |
|---------|------------|----------|------|---------------|
| `openaiCompatAdapter` | Ollama, Groq, Together, any OpenAI-compatible | `POST {base}/v1/chat/completions` | `Authorization: Bearer {key}` | `LLM_PROVIDER=openai-compat` |
| `openaiAdapter` | OpenAI (Responses API) | `POST https://api.openai.com/v1/responses` | `Authorization: Bearer {key}` | `LLM_PROVIDER=openai` |
| `anthropicAdapter` | Anthropic (Messages API) | `POST https://api.anthropic.com/v1/messages` | `x-api-key: {key}` + `anthropic-version: 2023-06-01` | `LLM_PROVIDER=anthropic` |
| `disabledAdapter` | None (no-op) | N/A | N/A | `LLM_PROVIDER=disabled` or unset |

### Provider Selection

```go
func NewNudgeLLM(provider string, cfg LLMConfig) (NudgeLLM, error) {
    switch strings.ToLower(provider) {
    case "openai-compat":
        return newOpenAICompatAdapter(cfg)
    case "openai":
        return newOpenAIAdapter(cfg)
    case "anthropic":
        return newAnthropicAdapter(cfg)
    case "disabled", "":
        return disabledAdapter{}, nil
    default:
        return nil, fmt.Errorf("unknown LLM provider: %q", provider)
    }
}
```

### Configuration

```go
type LLMConfig struct {
    BaseURL string // for openai-compat (e.g., http://localhost:11434/v1)
    APIKey  string // from LLM_API_KEY env var
    Model   string // from LLM_MODEL env var (e.g., gpt-4o-mini, claude-3-haiku)
    Timeout time.Duration // default 10s
}
```

Loaded from environment variables at startup: `LLM_PROVIDER`, `LLM_API_KEY`, `LLM_MODEL`, `LLM_BASE_URL` (for openai-compat only).

### No Third-Party SDKs

Each adapter uses raw `net/http` with typed request/response structs:

```go
type chatCompletionRequest struct {
    Model    string              `json:"model"`
    Messages []chatMessage       `json:"messages"`
    MaxTokens int                `json:"max_tokens,omitempty"`
}

type chatMessage struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

type chatCompletionResponse struct {
    Choices []struct {
        Message chatMessage `json:"message"`
    } `json:"choices"`
    Error *struct {
        Message string `json:"message"`
    } `json:"error,omitempty"`
}
```

Each adapter is ~50 lines: build request, set auth headers, call `http.Do`, parse response, extract text. Minimal, auditable, zero external dependencies.

### Graceful Fallback

On any LLM error (timeout, HTTP 4xx/5xx, rate limit, empty response), the adapter returns `("", error)`. The nudge engine treats an empty tip as a valid outcome — the core nudge (rule-based message) is still delivered. LLM failure never blocks or degrades the nudge experience.

## Alternatives Considered

**OpenAI Go SDK (`github.com/openai/openai-go`)** — Rejected. Vendor lock-in: only supports OpenAI. Transitive dependencies (54 packages in latest version). The SDK provides features (streaming, assistants, fine-tuning, file upload) that are irrelevant to Senpay's single use case (one-off text generation). Raw HTTP with typed structs is simpler and avoids dependency churn.

**LangChain Go (`github.com/tmc/langchaingo`)** — Rejected. Framework lock-in: LangChain imposes its own abstraction model (chains, agents, memory, tools). Massive dependency footprint (~200+ transitive packages). Severely over-engineered for a single `Generate(ctx, prompt) (string, error)` call. LangChain is appropriate for complex agent workflows, not simple prompt→response.

**Individual provider packages (separate packages per adapter: `llm/openai`, `llm/anthropic`)** — Rejected. Duplicates the interface definition across packages. A single `NudgeLLM` interface with adapters in the same package is simpler to use and test. The adapter implementations share common HTTP patterns (auth headers, error handling, timeout), which a single-package approach keeps DRY.

**gRPC-based LLM service abstraction** — Rejected. Adds network hop and gRPC infrastructure for a call that should be a simple HTTP request. Over-engineering for a single internal use case.

## Consequences

**Positive:**

- Zero SDK dependencies — no `openai-go`, no `langchaingo`, no framework lock-in
- Adding a new provider requires ~50 lines of Go (request struct, HTTP call, response parse)
- Single `NudgeLLM` interface enables easy mocking in tests (`mockAdapter` returns canned tips)
- Graceful fallback: LLM failure never affects core nudge delivery
- Raw `net/http` is auditable: every line of HTTP interaction is visible, no hidden SDK behavior
- Provider-agnostic: switching from OpenAI to Anthropic is an env var change, not a code change

**Negative:**

- Manual HTTP request construction (auth headers, JSON marshaling) is boilerplate — acceptable for 3 adapters × ~50 lines each
- No automatic retry, rate limit handling, or exponential backoff — must be implemented manually if needed
- No streaming support — acceptable for nudge tip generation (short responses, ~50 tokens)
- Typed response structs must be maintained in sync with provider API changes — low risk for stable APIs (chat/completions, messages)
- Provider-specific features (e.g., Anthropic's computer use, OpenAI's structured outputs) are not available — acceptable, not needed

## Compliance

All LLM calls must go through the `NudgeLLM` interface. No direct HTTP calls to LLM endpoints outside the adapter layer. Provider selection must use `LLM_PROVIDER` env var. Adapter implementations belong in `internal/senpai/llm/` (shell). The `NudgeLLM` interface is defined in `internal/senpai/store.go` alongside other interfaces per FCIS convention.
