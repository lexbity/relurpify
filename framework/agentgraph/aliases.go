package agentgraph

import (
	"codeburg.org/lexbit/relurpify/framework/core"
)

// Context is now a map-based state container
type Context = map[string]any

// NewContext creates a new empty context.
func NewContext() *Context {
	ctx := make(Context)
	return &ctx
}

// CloneContext creates a shallow copy of a context.
func CloneContext(ctx *Context) *Context {
	if ctx == nil {
		return NewContext()
	}
	clone := make(Context, len(*ctx))
	for k, v := range *ctx {
		clone[k] = v
	}
	return &clone
}

// ComputeBranchDelta computes the delta between two context states.
func ComputeBranchDelta(base, current *Context) BranchContextDelta {
	delta := BranchContextDelta{
		StateWrites: make(map[string]any),
	}
	if base == nil || current == nil {
		return delta
	}
	for key, value := range *current {
		if baseValue, ok := (*base)[key]; !ok || baseValue != value {
			delta.StateWrites[key] = value
		}
	}
	return delta
}

// ContextGet retrieves a value from the context.
func ContextGet(ctx *Context, key string) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	val, ok := (*ctx)[key]
	return val, ok
}

// ContextSet stores a value in the context.
func ContextSet(ctx *Context, key string, value any) {
	if ctx == nil {
		return
	}
	(*ctx)[key] = value
}

// GetContextSnapshot returns a point-in-time copy of context state.
func GetContextSnapshot(ctx *Context) map[string]any {
	if ctx == nil {
		return nil
	}
	snapshot := make(map[string]any, len(*ctx))
	for k, v := range *ctx {
		snapshot[k] = v
	}
	return snapshot
}

type Result = core.Result
type TaskType = core.TaskType
type TaskContext = core.TaskContext
type Telemetry = core.Telemetry
type Event = core.Event
type CheckpointTelemetry = core.CheckpointTelemetry
type LLMOptions = core.LLMOptions
type LanguageModel = core.LanguageModel
type LLMResponse = core.LLMResponse
type Message = core.Message
type ToolCall = core.ToolCall
type Tool = core.Tool

// ContextReference types for system_nodes.go compatibility
type ContextReference struct {
	Kind     ContextReferenceKind
	ID       string
	URI      string
	Version  string
	Detail   string
	Metadata map[string]string
}

type ContextReferenceKind string

const (
	ContextReferenceKindFile     ContextReferenceKind = "file"
	ContextReferenceKindSymbol   ContextReferenceKind = "symbol"
	ContextReferenceKindAnchor   ContextReferenceKind = "anchor"
	ContextReferenceKindMemory   ContextReferenceKind = "memory"
	ContextReferenceKindExternal ContextReferenceKind = "external"
)

// MemoryClass categorizes working memory entries (from framework/core)
type MemoryClass = core.MemoryClass

// Branch delta types are defined in branch_delta.go and available directly

const (
	EventGraphStart  = core.EventGraphStart
	EventGraphFinish = core.EventGraphFinish
	EventNodeStart   = core.EventNodeStart
	EventNodeFinish  = core.EventNodeFinish
	EventNodeError   = core.EventNodeError
)

var WithTaskContext = core.WithTaskContext
