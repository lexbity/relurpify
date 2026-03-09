package core

import "context"

// Tag constants classify tools for policy enforcement.
const (
	TagReadOnly    = "read-only"
	TagExecute     = "execute"
	TagDestructive = "destructive"
	TagNetwork     = "network"
)

// Tool defines local-native capabilities accessible to agents. Tool is no
// longer the umbrella term for every callable capability; provider-backed and
// Relurpic capabilities may also be callable without being tools. The gVisor
// sandbox and executable allowlist assumptions attach to this local-native
// runtime family, not to every callable capability in the framework.
type Tool interface {
	Name() string
	Description() string
	Category() string
	Parameters() []ToolParameter
	Execute(ctx context.Context, state *Context, args map[string]interface{}) (*ToolResult, error)
	IsAvailable(ctx context.Context, state *Context) bool
	Permissions() ToolPermissions
	Tags() []string
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

// CapabilityExecutionResult is the capability-native name for execution
// results. ToolResult remains during the migration because local tools are one
// callable capability family.
type CapabilityExecutionResult = ToolResult
