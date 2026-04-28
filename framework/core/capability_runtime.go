package core

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// CapabilityHandler is the canonical runtime contract for registry-managed
// capabilities. The handler is responsible for describing the capability in
// runtime terms instead of forcing the registry to depend on tool-specific APIs.
type CapabilityHandler interface {
	Descriptor(ctx context.Context, env *contextdata.Envelope) CapabilityDescriptor
}

// PromptMessage is a framework-owned prompt message shape used by runtime
// prompt capabilities without forcing MCP protocol types into the core model.
type PromptMessage struct {
	Role    string         `json:"role,omitempty" yaml:"role,omitempty"`
	Content []ContentBlock `json:"content,omitempty" yaml:"content,omitempty"`
}

// PromptRenderResult captures the rendered prompt payload for a prompt
// capability invocation.
type PromptRenderResult struct {
	Description string          `json:"description,omitempty" yaml:"description,omitempty"`
	Messages    []PromptMessage `json:"messages,omitempty" yaml:"messages,omitempty"`
	Metadata    map[string]any  `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// ResourceReadResult captures a framework-owned resource read response.
type ResourceReadResult struct {
	Contents []ContentBlock `json:"contents,omitempty" yaml:"contents,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// InvocableCapabilityHandler is implemented by capability kinds that can be
// directly executed by the framework.
type InvocableCapabilityHandler interface {
	CapabilityHandler
	Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*CapabilityExecutionResult, error)
}

// PromptCapabilityHandler is implemented by runtime-backed prompt capabilities.
type PromptCapabilityHandler interface {
	CapabilityHandler
	RenderPrompt(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*PromptRenderResult, error)
}

// ResourceCapabilityHandler is implemented by runtime-backed resource
// capabilities.
type ResourceCapabilityHandler interface {
	CapabilityHandler
	ReadResource(ctx context.Context, env *contextdata.Envelope) (*ResourceReadResult, error)
}

// AvailabilityAwareCapabilityHandler allows handlers to provide dynamic
// availability without exposing tool-specific hooks.
type AvailabilityAwareCapabilityHandler interface {
	CapabilityHandler
	Availability(ctx context.Context, env *contextdata.Envelope) AvailabilitySpec
}
