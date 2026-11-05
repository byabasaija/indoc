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

const (
	// DefaultOpenAIBaseURL is the standard OpenAI endpoint.
	DefaultOpenAIBaseURL = "https://api.openai.com/v1"

	// DefaultHTTPTimeout is the maximum duration for an LLM response.
	DefaultHTTPTimeout = 120 * time.Second
)

// OpenAIProvider implements Provider for OpenAI and any OpenAI-compatible endpoint
// (e.g., DeepSeek, Groq, OpenRouter, vLLM, Ollama).
type OpenAIProvider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewOpenAIProvider creates an OpenAIProvider instance.
// If baseURL is empty, it defaults to https://api.openai.com/v1.
func NewOpenAIProvider(apiKey, baseURL string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = DefaultOpenAIBaseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &OpenAIProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: DefaultHTTPTimeout,
		},
	}
}

// SetHTTPClient overrides the default HTTP client (useful for custom transports or tests).
func (p *OpenAIProvider) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.httpClient = client
	}
}

// openAIErrorResponse parses API error payloads.
type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// openAIChoice represents a single completion choice.
type openAIChoice struct {
	Index   int `json:"index"`
	Message struct {
		Role      Role       `json:"role"`
		Content   string     `json:"content"`
		ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	} `json:"message"`
	FinishReason string `json:"finish_reason"`
}

// openAIChatCompletionResponse represents the raw JSON from /chat/completions.
type openAIChatCompletionResponse struct {
	ID      string         `json:"id"`
	Choices []openAIChoice `json:"choices"`
	Usage   Usage          `json:"usage"`
}

// Chat executes a request against the /chat/completions endpoint.
func (p *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	url := fmt.Sprintf("%s/chat/completions", p.baseURL)

	payloadBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("LLM API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp openAIErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("LLM API error (status %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("LLM API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var completionResp openAIChatCompletionResponse
	if err := json.Unmarshal(respBody, &completionResp); err != nil {
		return nil, fmt.Errorf("failed to decode LLM response: %w", err)
	}

	if len(completionResp.Choices) == 0 {
		return nil, fmt.Errorf("LLM returned empty choices")
	}

	firstChoice := completionResp.Choices[0]
	return &ChatResponse{
		Content:   firstChoice.Message.Content,
		ToolCalls: firstChoice.Message.ToolCalls,
		Usage:     completionResp.Usage,
	}, nil
}
