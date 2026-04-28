package agentgraph

import (
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// Envelope is the execution context passed to graph nodes.
// This is a type alias for contextdata.Envelope to provide a convenient
// import path for agentgraph consumers.
type Envelope = contextdata.Envelope

// BranchDelta tracks mutations made on a branch.
// This is a type alias for contextdata.BranchDelta.
type BranchDelta = contextdata.BranchDelta

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

// Branch delta types are now provided by contextdata.BranchDelta

const (
	EventGraphStart  = core.EventGraphStart
	EventGraphFinish = core.EventGraphFinish
	EventNodeStart   = core.EventNodeStart
	EventNodeFinish  = core.EventNodeFinish
	EventNodeError   = core.EventNodeError
)

var WithTaskContext = core.WithTaskContext
