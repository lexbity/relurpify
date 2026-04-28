package core

import (
	"context"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// Re-export tool contracts from platform/contracts for backward compatibility.
// These type aliases allow existing code to continue using core.Tool, core.ToolParameter, etc.

// Tag constants classify tools for policy enforcement.
const (
	TagReadOnly    = contracts.TagReadOnly
	TagExecute     = contracts.TagExecute
	TagDestructive = contracts.TagDestructive
	TagNetwork     = contracts.TagNetwork
)

// Tool defines local-native capabilities accessible to agents.
type Tool = contracts.Tool

// ToolParameter describes an argument the tool accepts.
type ToolParameter = contracts.ToolParameter

// ToolResult is returned by every tool execution.
type ToolResult = contracts.ToolResult

// CapabilityExecutionResult is the capability-native name for execution results.
type CapabilityExecutionResult = contracts.CapabilityExecutionResult

// NewToolResult creates a ToolResult with the provided data.
func NewToolResult(data map[string]interface{}) *ToolResult {
	return &ToolResult{
		Success: true,
		Data:    data,
	}
}

// NewToolResultWithError creates a failed ToolResult with an error message.
func NewToolResultWithError(err string) *ToolResult {
	return &ToolResult{
		Success: false,
		Error:   err,
	}
}

// ToolResultFromContext creates a ToolResult from execution context.
func ToolResultFromContext(ctx context.Context, data map[string]interface{}) *ToolResult {
	_ = ctx // reserved for future context-aware behavior
	return NewToolResult(data)
}

// BackendClass is re-exported from platform/contracts.
type BackendClass = contracts.BackendClass

const (
	BackendClassTransport = contracts.BackendClassTransport
	BackendClassNative    = contracts.BackendClassNative
)

// BackendCapabilities is re-exported from platform/contracts.
type BackendCapabilities = contracts.BackendCapabilities
