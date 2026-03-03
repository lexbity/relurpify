package core

import "context"

// LLMOptions configures language model calls. Keeping the options struct inside
// the core package avoids hard-coding provider-specific fields in agent code.
type LLMOptions struct {
	Model          string
	Temperature    float64
	MaxTokens      int
	Stop           []string
	TopP           float64
	Stream         bool
	StreamCallback func(string) `json:"-"`
}

// ToolCall encodes a function invocation requested by the LLM.
type ToolCall struct {
	ID   string                 `json:"id,omitempty"`
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

// LLMResponse is the result of a language model invocation.
type LLMResponse struct {
	Text         string         `json:"text,omitempty"`
	FinishReason string         `json:"finish_reason,omitempty"`
	Usage        map[string]int `json:"usage,omitempty"`
	ToolCalls    []ToolCall     `json:"tool_calls,omitempty"`
}

// Message is used for chat-like interactions.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// LanguageModel provides the required LLM capabilities.
type LanguageModel interface {
	Generate(ctx context.Context, prompt string, options *LLMOptions) (*LLMResponse, error)
	GenerateStream(ctx context.Context, prompt string, options *LLMOptions) (<-chan string, error)
	Chat(ctx context.Context, messages []Message, options *LLMOptions) (*LLMResponse, error)
	ChatWithTools(ctx context.Context, messages []Message, tools []Tool, options *LLMOptions) (*LLMResponse, error)
}
