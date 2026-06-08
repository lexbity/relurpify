package capability

import (
	"context"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

type CapabilityHandler interface {
	Descriptor(ctx context.Context, env ports.State) CapabilityDescriptor
}

type PromptMessage struct {
	Role    string         `json:"role,omitempty"`
	Content []ContentBlock `json:"content,omitempty"`
}

type PromptRenderResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages,omitempty"`
	Metadata    map[string]any  `json:"metadata,omitempty"`
}

type ResourceReadResult struct {
	Contents []ContentBlock `json:"contents,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type InvocableCapabilityHandler interface {
	CapabilityHandler
	Invoke(ctx context.Context, env ports.State, args map[string]interface{}) (*ports.ToolResult, error)
}

type PromptCapabilityHandler interface {
	CapabilityHandler
	RenderPrompt(ctx context.Context, env ports.State, args map[string]interface{}) (*PromptRenderResult, error)
}

type ResourceCapabilityHandler interface {
	CapabilityHandler
	ReadResource(ctx context.Context, env ports.State) (*ResourceReadResult, error)
}

type AvailabilityAwareCapabilityHandler interface {
	CapabilityHandler
	Availability(ctx context.Context, env ports.State) AvailabilitySpec
}

type BackgroundInvocationHandle struct {
	JobID       string `json:"job_id"`
	Queue       string `json:"queue"`
	Kind        string `json:"kind"`
	SubmittedAt string `json:"submitted_at"`
}

type BackgroundCapabilityHandler interface {
	CapabilityHandler
	InvokeBackground(ctx context.Context, env ports.State, args map[string]interface{}) (*BackgroundInvocationHandle, error)
}
