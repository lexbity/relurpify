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

// LLMToolSpec is the provider-agnostic tool definition passed to LLM backends.
// It carries only the fields needed to describe a callable tool to a language
// model: name, description, and input schema. Provider-specific wire formats
// (e.g. Ollama's {"type":"function","function":{...}}) are handled entirely
// inside the platform/llm layer and do not leak into the capability model.
type LLMToolSpec struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	InputSchema *Schema `json:"input_schema,omitempty"`
}

// LLMToolSpecFromTool builds an LLMToolSpec from a local Tool implementation.
func LLMToolSpecFromTool(t Tool) LLMToolSpec {
	spec := LLMToolSpec{
		Name:        t.Name(),
		Description: t.Description(),
	}
	params := t.Parameters()
	if len(params) > 0 {
		props := make(map[string]*Schema, len(params))
		var required []string
		for _, p := range params {
			prop := &Schema{
				Type:        p.Type,
				Description: p.Description,
			}
			if p.Default != nil {
				prop.Default = p.Default
			}
			props[p.Name] = prop
			if p.Required {
				required = append(required, p.Name)
			}
		}
		spec.InputSchema = &Schema{
			Type:       "object",
			Properties: props,
			Required:   required,
		}
	}
	return spec
}

// LLMToolSpecFromDescriptor builds an LLMToolSpec from a CapabilityDescriptor.
// Used for non-local capabilities (provider-backed, Relurpic) that are callable
// by the LLM but are not local Tool implementations.
func LLMToolSpecFromDescriptor(d CapabilityDescriptor) LLMToolSpec {
	name := d.Name
	if name == "" {
		name = d.ID
	}
	return LLMToolSpec{
		Name:        name,
		Description: d.Description,
		InputSchema: d.InputSchema,
	}
}

// LLMToolSpecsFromTools converts a slice of local Tool implementations to
// LLMToolSpec values for passing to ChatWithTools.
func LLMToolSpecsFromTools(tools []Tool) []LLMToolSpec {
	if len(tools) == 0 {
		return nil
	}
	specs := make([]LLMToolSpec, len(tools))
	for i, t := range tools {
		specs[i] = LLMToolSpecFromTool(t)
	}
	return specs
}

// ProfiledModel is an optional extension for LanguageModel implementations that
// expose active model profile metadata. Callers type-assert to check support:
//
//   if pm, ok := model.(ProfiledModel); ok { ... }
//
// The LanguageModel interface is not changed.
type ProfiledModel interface {
	ToolRepairStrategy() string // "llm" | "heuristic-only"
	MaxToolsPerCall() int       // 0 = no limit
}

// LanguageModel provides the required LLM capabilities.
type LanguageModel interface {
	Generate(ctx context.Context, prompt string, options *LLMOptions) (*LLMResponse, error)
	GenerateStream(ctx context.Context, prompt string, options *LLMOptions) (<-chan string, error)
	Chat(ctx context.Context, messages []Message, options *LLMOptions) (*LLMResponse, error)
	ChatWithTools(ctx context.Context, messages []Message, tools []LLMToolSpec, options *LLMOptions) (*LLMResponse, error)
}
