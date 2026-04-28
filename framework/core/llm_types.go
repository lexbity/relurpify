package core

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// Re-export LLM types from platform/contracts for backward compatibility.
// These type aliases allow existing code to continue using core.Message,
// core.LLMOptions, etc. while the canonical definitions live in platform/contracts.

// LLMOptions configures language model calls.
type LLMOptions = contracts.LLMOptions

// ToolCall encodes a function invocation requested by the LLM.
type ToolCall = contracts.ToolCall

// LLMResponse is the result of a language model invocation.
type LLMResponse = contracts.LLMResponse

// Message is used for chat-like interactions.
type Message = contracts.Message

// LLMToolSpec is the provider-agnostic tool definition passed to LLM backends.
type LLMToolSpec = contracts.LLMToolSpec

// ProfiledModel is an optional extension for LanguageModel implementations.
type ProfiledModel = contracts.ProfiledModel

// LanguageModel provides the required LLM capabilities.
type LanguageModel = contracts.LanguageModel

// LLMToolSpecFromTool builds an LLMToolSpec from a local Tool implementation.
func LLMToolSpecFromTool(t Tool) LLMToolSpec {
	return contracts.LLMToolSpecFromTool(t)
}

// LLMToolSpecFromDescriptor builds an LLMToolSpec from a CapabilityDescriptor.
// This function extracts only the fields needed for LLM tool calling (Name,
// Description, InputSchema) since the full CapabilityDescriptor type remains
// in framework/core.
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
	return contracts.LLMToolSpecsFromTools(tools)
}
