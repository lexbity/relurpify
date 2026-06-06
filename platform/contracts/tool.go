// Package contracts defines the core interfaces and types for platform tools.
// These contracts allow platform tools to be implemented without importing
// the framework package, enabling proper dependency direction (framework -> platform).
package contracts

import (
	"time"

	"codeburg.org/lexbit/relurpify/framework/capability/ports"
)

// ToolParameterType enumerates the supported JSON Schema-like parameter types
// for tool definitions.
type ToolParameterType = ports.ToolParameterType

const (
	ToolParamString  = ports.ToolParamString
	ToolParamInteger = ports.ToolParamInteger
	ToolParamNumber  = ports.ToolParamNumber
	ToolParamBoolean = ports.ToolParamBoolean
	ToolParamArray   = ports.ToolParamArray
	ToolParamObject  = ports.ToolParamObject
)

// ToolChunk represents a single piece of streaming output from a tool that
// implements the StreamingTool interface. Chunks are delivered in order over
// a channel; the final chunk carries Done=true.
type ToolChunk = ports.ToolChunk

// StreamingTool is an optional extension for Tool implementations that support
// streaming their output incrementally. Use a type assertion to check:
//
//	if st, ok := tool.(StreamingTool); ok { ... }
type StreamingTool = ports.StreamingTool

// RollbackToken identifies a completed tool invocation for rollback.
type RollbackToken = ports.RollbackToken

// RevertibleTool is an optional extension for Tool implementations that support
// undoing or compensating for a completed execution. Register the rollback
// token returned by InvokeCapability and call RollbackCapability to undo.
//
//	if rt, ok := tool.(RevertibleTool); ok { rt.Rollback(ctx, token) }
type RevertibleTool = ports.RevertibleTool

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
type Tool = ports.Tool

// ToolParameter describes an argument the tool accepts.
type ToolParameter = ports.ToolParameter

// ToolResult is returned by every tool execution.
type ToolResult = ports.ToolResult

// CapabilityExecutionResult is the capability-native name for execution
// results. ToolResult remains during the migration because local tools are one
// callable capability family.
type CapabilityExecutionResult = ToolResult

// ToolPermissions summarises tool requirements.
type ToolPermissions = ports.ToolPermissions

// ParamKeysProvider is an optional interface for go_native tools that
// declares which manifest parameter keys they consume. The toolcapabilities
// build process uses this to assert that consumed keys exist in the manifest,
// providing compile-time drift detection.
type ParamKeysProvider = ports.ParamKeysProvider

// CommandPreset describes a reusable command wrapper for shell tools.
// It is kept here (rather than in platform/shell/execute) so that the
// query/catalog layer can reference it without importing legacy executor code.
type CommandPreset struct {
	Name        string
	Command     string
	DefaultArgs []string
	Description string
	Category    string
	Tags        []string
	Timeout     time.Duration
	AllowStdin  bool
	AllowFlags  bool
	WorkdirMode string
}

// NewCommandPreset normalizes a CommandPreset with sensible defaults.
func NewCommandPreset(p CommandPreset) CommandPreset {
	if p.Category == "" {
		p.Category = "cli"
	}
	if p.Timeout <= 0 {
		p.Timeout = 60 * time.Second
	}
	if p.WorkdirMode == "" {
		p.WorkdirMode = "workspace"
	}
	return p
}
