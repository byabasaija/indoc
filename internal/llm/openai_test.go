package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// roundTripFunc allows in-memory mocking of http.RoundTripper without opening TCP sockets.
type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func newMockClient(fn roundTripFunc) *http.Client {
	return &http.Client{
		Transport: fn,
	}
}

func TestMessageHelpers(t *testing.T) {
	sys := SystemMessage("system prompt")
	if sys.Role != RoleSystem || sys.Content != "system prompt" {
		t.Errorf("unexpected system message: %+v", sys)
	}

	user := UserMessage("user prompt")
	if user.Role != RoleUser || user.Content != "user prompt" {
		t.Errorf("unexpected user message: %+v", user)
	}

	toolMsg := ToolResultMessage("call_1", "tool output")
	if toolMsg.Role != RoleTool || toolMsg.ToolCallID != "call_1" || toolMsg.Content != "tool output" {
		t.Errorf("unexpected tool message: %+v", toolMsg)
	}
}

func TestOpenAIChatSuccess(t *testing.T) {
	mockClient := newMockClient(func(r *http.Request) *http.Response {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
		}

		resp := openAIChatCompletionResponse{
			ID: "chatcmpl-123",
			Choices: []openAIChoice{
				{
					Index: 0,
					Message: struct {
						Role      Role       `json:"role"`
						Content   string     `json:"content"`
						ToolCalls []ToolCall `json:"tool_calls,omitempty"`
					}{
						Role:    RoleAssistant,
						Content: "Documentation generated successfully",
					},
					FinishReason: "stop",
				},
			},
			Usage: Usage{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
			},
		}

		bodyBytes, _ := json.Marshal(resp)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
		}
	})

	provider := NewOpenAIProvider("test-key", "https://api.openai.com/v1")
	provider.SetHTTPClient(mockClient)

	res, err := provider.Chat(context.Background(), ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []Message{UserMessage("Hello")},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Content != "Documentation generated successfully" {
		t.Errorf("unexpected content: %s", res.Content)
	}
	if res.Usage.TotalTokens != 30 {
		t.Errorf("expected 30 total tokens, got %d", res.Usage.TotalTokens)
	}
}

func TestOpenAIToolCallParsing(t *testing.T) {
	mockClient := newMockClient(func(r *http.Request) *http.Response {
		resp := openAIChatCompletionResponse{
			ID: "chatcmpl-tools",
			Choices: []openAIChoice{
				{
					Index: 0,
					Message: struct {
						Role      Role       `json:"role"`
						Content   string     `json:"content"`
						ToolCalls []ToolCall `json:"tool_calls,omitempty"`
					}{
						Role:    RoleAssistant,
						Content: "",
						ToolCalls: []ToolCall{
							{
								ID:   "call_999",
								Type: "function",
								Function: FunctionCall{
									Name:      "read_source_file",
									Arguments: `{"file_path": "models/user.go"}`,
								},
							},
						},
					},
					FinishReason: "tool_calls",
				},
			},
		}

		bodyBytes, _ := json.Marshal(resp)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
		}
	})

	provider := NewOpenAIProvider("test-key", "https://api.openai.com/v1")
	provider.SetHTTPClient(mockClient)

	res, err := provider.Chat(context.Background(), ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []Message{UserMessage("Inspect user model")},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(res.ToolCalls))
	}
	if res.ToolCalls[0].Function.Name != "read_source_file" {
		t.Errorf("expected tool name 'read_source_file', got %s", res.ToolCalls[0].Function.Name)
	}
}

func TestOpenAIErrorHandling(t *testing.T) {
	mockClient := newMockClient(func(r *http.Request) *http.Response {
		body := `{"error":{"message":"Incorrect API key provided","type":"invalid_request_error"}}`
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader([]byte(body))),
		}
	})

	provider := NewOpenAIProvider("invalid-key", "https://api.openai.com/v1")
	provider.SetHTTPClient(mockClient)

	_, err := provider.Chat(context.Background(), ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []Message{UserMessage("Hello")},
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
