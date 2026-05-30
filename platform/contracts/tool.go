// Package contracts defines the core interfaces and types for platform tools.
// These contracts allow platform tools to be implemented without importing
// the framework package, enabling proper dependency direction (framework -> platform).
package contracts

import (
	"context"
	"errors"
)

// ToolParameterType enumerates the supported JSON Schema-like parameter types
// that a tool can declare for its arguments.
type ToolParameterType string

const (
	ToolParamString  ToolParameterType = "string"
	ToolParamInteger ToolParameterType = "integer"
	ToolParamNumber  ToolParameterType = "number"
	ToolParamBoolean ToolParameterType = "boolean"
	ToolParamArray   ToolParameterType = "array"
	ToolParamObject  ToolParameterType = "object"
)

// ToolChunk represents a single piece of streaming output from a tool that
// implements the StreamingTool interface. Chunks are delivered in order over
// a channel; the final chunk carries Done=true.
type ToolChunk struct {
	Data   map[string]interface{} `json:"data,omitempty"`
	Error  string                 `json:"error,omitempty"`
	Done   bool                   `json:"done"`
	SeqNum int                    `json:"seq_num,omitempty"`
}

// StreamingTool is an optional extension for Tool implementations that support
// streaming their output incrementally. Use a type assertion to check:
//
//	if st, ok := tool.(StreamingTool); ok { ... }
type StreamingTool interface {
	Tool
	ExecuteStream(ctx context.Context, args map[string]interface{}) (<-chan ToolChunk, error)
}

// RollbackToken identifies a completed tool invocation for rollback.
type RollbackToken struct {
	InvocationID string
	ToolName     string
	Args         map[string]interface{}
	Result       *ToolResult
}

// RevertibleTool is an optional extension for Tool implementations that support
// undoing or compensating for a completed execution. Register the rollback
// token returned by InvokeCapability and call RollbackCapability to undo.
//
//	if rt, ok := tool.(RevertibleTool); ok { rt.Rollback(ctx, token) }
type RevertibleTool interface {
	Tool
	// Rollback undoes the effect of a previous Execute call identified by
	// the given token. Implementations should be idempotent.
	Rollback(ctx context.Context, token RollbackToken) error
}

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
	Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error)
	IsAvailable(ctx context.Context) bool
	Permissions() ToolPermissions
	Tags() []string
}

// ToolParameter describes an argument the tool accepts.
type ToolParameter struct {
	Name        string
	Type        ToolParameterType
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

// ToolPermissions summarises tool requirements.
type ToolPermissions struct {
	Permissions *PermissionSet
}

// ParamKeysProvider is an optional interface for go_native tools that
// declares which manifest parameter keys they consume. The toolcapabilities
// build process uses this to assert that consumed keys exist in the manifest,
// providing compile-time drift detection.
type ParamKeysProvider interface {
	// ParamKeys returns the parameter names this tool reads from the
	// invocation args map. Used for registration-time assertion against
	// the manifest's declared parameters.
	ParamKeys() []string
}

// Validate ensures tool permission manifests are well-formed.
func (t ToolPermissions) Validate() error {
	if t.Permissions == nil {
		return errors.New("tool permissions missing")
	}
	return t.Permissions.Validate()
}
