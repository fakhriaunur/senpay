package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// openAICompatibleAdapter implements NudgeLLM for any OpenAI-compatible API
// (Ollama, Groq, Together, Gemini via /v1beta/openai, etc.) using
// POST {BaseURL}/v1/chat/completions.
type openAICompatibleAdapter struct {
	config LLMConfig
}

// chatCompletionRequest is the request body for POST /v1/chat/completions.
type chatCompletionRequest struct {
	Model    string              `json:"model"`
	Messages []chatMessage       `json:"messages"`
}

// chatMessage is a message in the chat completion request.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionResponse is the response body from POST /v1/chat/completions.
type chatCompletionResponse struct {
	Choices []chatChoice `json:"choices"`
}

// chatChoice is a choice in the chat completion response.
type chatChoice struct {
	Message chatResponseMessage `json:"message"`
}

// chatResponseMessage is a response message.
type chatResponseMessage struct {
	Content string `json:"content"`
}

// Generate sends a prompt to the OpenAI-compatible API and returns the generated text.
func (a *openAICompatibleAdapter) Generate(ctx context.Context, prompt string) (string, error) {
	baseURL := strings.TrimRight(a.config.BaseURL, "/")
	url := baseURL + "/v1/chat/completions"

	body := chatCompletionRequest{
		Model: a.config.Model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("llm openai-compatible: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("llm openai-compatible: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.config.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm openai-compatible: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("llm openai-compatible: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm openai-compatible: API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatCompletionResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("llm openai-compatible: parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("llm openai-compatible: empty choices in response")
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}
