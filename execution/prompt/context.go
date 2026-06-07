package prompt

import (
	"strings"

	capability "codeburg.org/lexbit/relurpify/capability"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
)

// RuntimeContext is the full input to Resolve. All fields are optional unless
// noted. Providers receive a copy of this struct and must not retain references
// to Envelope after their Provide call returns.
type RuntimeContext struct {
	// Core resolution inputs.
	Variables  map[string]string // runtime variable overrides
	State      map[string]any    // evaluated by when-expressions
	Envelope   *contextdata.Envelope
	Paradigm   string // consuming paradigm name, e.g. "react"
	ConsumerID string // agent or capability id invoking resolve

	// Extended: available to providers that need more than the envelope.
	Task         *execution.Task
	Tools        []ports.Tool
	Capabilities []capability.CapabilityDescriptor
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

// NewRuntimeContext creates a runtime context with initialized maps.
func NewRuntimeContext(env *contextdata.Envelope, paradigm, consumerID string) RuntimeContext {
	return RuntimeContext{
		Variables:  make(map[string]string),
		State:      make(map[string]any),
		Envelope:   env,
		Paradigm:   strings.TrimSpace(paradigm),
		ConsumerID: strings.TrimSpace(consumerID),
	}
}

// Clone returns a deep copy of the runtime context maps while preserving the
// envelope and extended references.
func (ctx RuntimeContext) Clone() RuntimeContext {
	out := ctx
	if len(ctx.Variables) > 0 {
		out.Variables = make(map[string]string, len(ctx.Variables))
		for k, v := range ctx.Variables {
			out.Variables[k] = v
		}
	} else if ctx.Variables != nil {
		out.Variables = make(map[string]string)
	}
	if len(ctx.State) > 0 {
		out.State = make(map[string]any, len(ctx.State))
		for k, v := range ctx.State {
			out.State[k] = v
		}
	} else if ctx.State != nil {
		out.State = make(map[string]any)
	}
	return out
}

// WithVariable returns a copy of the runtime context with one variable set.
func (ctx RuntimeContext) WithVariable(key, value string) RuntimeContext {
	if ctx.Variables == nil {
		ctx.Variables = make(map[string]string)
	}
	ctx.Variables[key] = value
	return ctx
}

// WithStateValue returns a copy of the runtime context with one state value set.
func (ctx RuntimeContext) WithStateValue(key string, value any) RuntimeContext {
	if ctx.State == nil {
		ctx.State = make(map[string]any)
	}
	ctx.State[key] = value
	return ctx
}

// WithStateMap returns a copy of the runtime context with the provided state merged in.
func (ctx RuntimeContext) WithStateMap(values map[string]any) RuntimeContext {
	if len(values) == 0 {
		return ctx
	}
	if ctx.State == nil {
		ctx.State = make(map[string]any, len(values))
	}
	for k, v := range values {
		ctx.State[k] = v
	}
	return ctx
}
