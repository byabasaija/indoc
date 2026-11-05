package llm

import (
	"context"
)

// Provider abstracts any LLM endpoint (OpenAI, Anthropic, Ollama, Gemini, etc.).
type Provider interface {
	// Chat executes a conversational turn with optional tool support.
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}

// SystemMessage creates a new system-role message.
func SystemMessage(content string) Message {
	return Message{
		Role:    RoleSystem,
		Content: content,
	}
}

// UserMessage creates a new user-role message.
func UserMessage(content string) Message {
	return Message{
		Role:    RoleUser,
		Content: content,
	}
}

// AssistantMessage creates an assistant text response message.
func AssistantMessage(content string) Message {
	return Message{
		Role:    RoleAssistant,
		Content: content,
	}
}

// AssistantToolCallsMessage creates an assistant message requesting tool calls.
func AssistantToolCallsMessage(toolCalls []ToolCall) Message {
	return Message{
		Role:      RoleAssistant,
		ToolCalls: toolCalls,
	}
}

// ToolResultMessage creates a message containing the output of a completed tool call.
func ToolResultMessage(toolCallID, content string) Message {
	return Message{
		Role:       RoleTool,
		ToolCallID: toolCallID,
		Content:    content,
	}
}
