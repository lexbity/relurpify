package prompt

import (
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// RuntimeContext is the full input to Resolve. All fields are optional unless
// noted. Providers receive a copy of this struct and must not retain references
// to Envelope after their Provide call returns.
type RuntimeContext struct {
	// Core resolution inputs.
	Variables  map[string]string  // runtime variable overrides
	State      map[string]any     // evaluated by when-expressions
	Envelope   *contextdata.Envelope
	Paradigm   string // consuming paradigm name, e.g. "react"
	ConsumerID string // agent or capability id invoking resolve

	// Extended: available to providers that need more than the envelope.
	Task         *core.Task
	Tools        []contracts.Tool
	Capabilities []core.CapabilityDescriptor
	AgentSpec    *agentspec.AgentRuntimeSpec
}

// ContextChunk is the runtime-supplied content for a SourceProvider block.
// An empty Content means the block is excluded from assembly.
type ContextChunk struct {
	Content string
}

// ContextProvider supplies content for a SourceProvider block at resolve time.
// All implementations must be safe for concurrent invocation.
type ContextProvider interface {
	Provide(ctx RuntimeContext) ContextChunk
}

// DescribingProvider is an optional extension of ContextProvider.
// Providers that implement it supply metadata for tooling, registry introspection,
// and validation panels.
type DescribingProvider interface {
	ContextProvider
	Describe() ProviderMetadata
}

// FailableProvider is an optional extension for providers that can signal errors.
// The resolver detects this interface at registration time and calls ProvideOrFail
// instead of Provide.
type FailableProvider interface {
	ContextProvider
	ProvideOrFail(ctx RuntimeContext) (ContextChunk, error)
}

// ProviderMetadata describes a registered context provider for tooling.
type ProviderMetadata struct {
	Name        string
	Description string
	Paradigms   []string // empty = any paradigm
	ReadsKeys   []string // envelope keys read (for static analysis tooling)
}
