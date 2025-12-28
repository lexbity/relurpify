package core

import "context"

// Tool defines capabilities accessible to agents. Each implementation can wrap
// anything from a filesystem helper to an LSP proxy. The metadata doubles as a
// schema that LLMs can reason about when deciding which tool to call.
type Tool interface {
	Name() string
	Description() string
	Category() string
	Parameters() []ToolParameter
	Execute(ctx context.Context, state *Context, args map[string]interface{}) (*ToolResult, error)
	IsAvailable(ctx context.Context, state *Context) bool
	Permissions() ToolPermissions
}

// ToolParameter describes an argument the tool accepts.
type ToolParameter struct {
	Name        string
	Type        string
	Description string
	Required    bool
	Default     interface{}
}

// ToolResult is returned by every tool execution.
type ToolResult struct {
	Success  bool
	Data     map[string]interface{}
	Error    string
	Metadata map[string]interface{}
}
