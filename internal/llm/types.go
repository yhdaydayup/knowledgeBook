package llm

import "context"

type GenerateJSONRequest struct {
	TaskName       string
	SystemPrompt   string
	UserPrompt     string
	Temperature    float64
	MaxTokens      int
	ResponseSchema []byte
}

// ChatMessage is a single message in a multi-turn conversation.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall represents the model requesting a function call.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall names the function and provides JSON-encoded arguments.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDefinition describes one tool available to the model.
type ToolDefinition struct {
	Type     string          `json:"type"` // "function"
	Function ToolFunctionDef `json:"function"`
}

// ToolFunctionDef is the function metadata within a ToolDefinition.
type ToolFunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ChatRequest is the input to a multi-turn chat completion call.
type ChatRequest struct {
	Messages    []ChatMessage    `json:"messages"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
}

// ChatResponse captures the model's reply or tool-call requests.
type ChatResponse struct {
	Content      string     `json:"content"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason"` // "stop" | "tool_calls"
}

type Client interface {
	Enabled() bool
	GenerateJSON(ctx context.Context, req GenerateJSONRequest) ([]byte, error)
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}
