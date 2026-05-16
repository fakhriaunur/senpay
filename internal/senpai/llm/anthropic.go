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

// anthropicAdapter implements NudgeLLM for the Anthropic Messages API
// using POST {BaseURL}/v1/messages.
type anthropicAdapter struct {
	config LLMConfig
}

// anthropicMessageRequest is the request body for POST /v1/messages.
type anthropicMessageRequest struct {
	Model     string                `json:"model"`
	MaxTokens int                   `json:"max_tokens"`
	Messages  []anthropicMsg        `json:"messages"`
}

// anthropicMsg is a message in the request.
type anthropicMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicMessageResponse is the response body from POST /v1/messages.
type anthropicMessageResponse struct {
	Content []anthropicContentBlock `json:"content"`
}

// anthropicContentBlock is a content block in the response.
type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Generate sends a prompt to the Anthropic Messages API and returns the generated text.
func (a *anthropicAdapter) Generate(ctx context.Context, prompt string) (string, error) {
	baseURL := strings.TrimRight(a.config.BaseURL, "/")
	url := baseURL + "/v1/messages"

	body := anthropicMessageRequest{
		Model:     a.config.Model,
		MaxTokens: 1024,
		Messages: []anthropicMsg{
			{Role: "user", Content: prompt},
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("llm anthropic: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("llm anthropic: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm anthropic: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("llm anthropic: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm anthropic: API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var msgResp anthropicMessageResponse
	if err := json.Unmarshal(respBody, &msgResp); err != nil {
		return "", fmt.Errorf("llm anthropic: parse response: %w", err)
	}

	// Collect all text content blocks.
	var texts []string
	for _, block := range msgResp.Content {
		if block.Type == "text" {
			texts = append(texts, block.Text)
		}
	}

	if len(texts) == 0 {
		return "", fmt.Errorf("llm anthropic: no text content in response")
	}

	return strings.TrimSpace(strings.Join(texts, "\n")), nil
}
