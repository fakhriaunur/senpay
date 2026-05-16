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

// openAIAdapter implements NudgeLLM for the newer OpenAI Responses API
// using POST {BaseURL}/v1/responses.
type openAIAdapter struct {
	config LLMConfig
}

// responseRequest is the request body for POST /v1/responses.
type responseRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

// responseOutput is an item in the output array.
type responseOutput struct {
	Content []responseContent `json:"content"`
}

// responseContent is a content block.
type responseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// responseResponse is the response body from POST /v1/responses.
type responseResponse struct {
	Output []responseOutput `json:"output"`
}

// Generate sends a prompt to the OpenAI Responses API and returns the generated text.
func (a *openAIAdapter) Generate(ctx context.Context, prompt string) (string, error) {
	baseURL := strings.TrimRight(a.config.BaseURL, "/")
	url := baseURL + "/v1/responses"

	body := responseRequest{
		Model: a.config.Model,
		Input: prompt,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("llm openai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("llm openai: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.config.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm openai: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("llm openai: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm openai: API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var respData responseResponse
	if err := json.Unmarshal(respBody, &respData); err != nil {
		return "", fmt.Errorf("llm openai: parse response: %w", err)
	}

	if len(respData.Output) == 0 {
		return "", fmt.Errorf("llm openai: empty output in response")
	}

	// Collect all text content blocks.
	var texts []string
	for _, out := range respData.Output {
		for _, c := range out.Content {
			if c.Type == "output_text" || c.Text != "" {
				texts = append(texts, c.Text)
			}
		}
	}

	if len(texts) == 0 {
		return "", fmt.Errorf("llm openai: no text content in output")
	}

	return strings.TrimSpace(strings.Join(texts, "\n")), nil
}
