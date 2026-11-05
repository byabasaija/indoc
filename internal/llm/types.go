package llm

// Role identifies who sent a message in the conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// FunctionDefinition describes a callable tool provided to the LLM.
type FunctionDefinition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"` // JSON Schema map or struct
}

// Tool represents a capability the model can invoke.
type Tool struct {
	Type     string             `json:"type"` // Standard is "function"
	Function FunctionDefinition `json:"function"`
}

// FunctionCall contains the specific function invocation requested by the model.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // Raw JSON arguments string
}

// ToolCall represents an individual tool call initiated by the model.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // Standard is "function"
	Function FunctionCall `json:"function"`
}

// Message is an individual message in a chat conversation.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // Required for RoleTool
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // Populated when RoleAssistant calls a tool
}

// Usage tracks token consumption across API calls.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ResponseFormat allows requesting structured output (e.g., json_object).
type ResponseFormat struct {
	Type string `json:"type"` // e.g. "json_object" or "text"
}

// ChatRequest bundles all parameters for an LLM conversation turn.
type ChatRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	Tools          []Tool          `json:"tools,omitempty"`
	Temperature    *float32        `json:"temperature,omitempty"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
}

// ChatResponse holds the LLM's response and usage statistics.
type ChatResponse struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Usage     Usage      `json:"usage"`
}
