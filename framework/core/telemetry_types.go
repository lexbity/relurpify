package core

import "time"

// EventType categorizes telemetry events.
type EventType string

const (
	EventGraphStart         EventType = "graph_start"
	EventGraphFinish        EventType = "graph_finish"
	EventNodeStart          EventType = "node_start"
	EventNodeFinish         EventType = "node_finish"
	EventNodeError          EventType = "node_error"
	EventAgentStart         EventType = "agent_start"
	EventAgentFinish        EventType = "agent_finish"
	EventLLMPrompt          EventType = "llm_prompt"
	EventLLMResponse        EventType = "llm_response"
	EventDelegationStart    EventType = "delegation_start"
	EventDelegationFinish   EventType = "delegation_finish"
	EventDelegationCancel   EventType = "delegation_cancel"
	EventCapabilityCall     EventType = "capability_call"
	EventCapabilityResult   EventType = "capability_result"
	EventToolCall           EventType = "tool_call"
	EventToolResult         EventType = "tool_result"
	EventStateChange        EventType = "state_change"
	EventInferenceError     EventType = "inference_error"
	EventInferenceTimeout   EventType = "inference_timeout"
	EventInferenceAbort     EventType = "inference_abort"
	EventBackendStateChange EventType = "backend_state_change"
	EventBackendWarm        EventType = "backend_warm"
	EventBackendClose       EventType = "backend_close"
	EventBackendRestart     EventType = "backend_restart"
)

// Event captures structured telemetry data.
type Event struct {
	Type      EventType              `json:"type"`
	NodeID    string                 `json:"node_id,omitempty"`
	TaskID    string                 `json:"task_id,omitempty"`
	Message   string                 `json:"message,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Telemetry captures execution traces emitted by the graph runtime.
type Telemetry interface {
	Emit(event Event)
}

// BudgetTelemetry extends telemetry with budget-management signals.
type BudgetTelemetry interface {
	OnArtifactCompression(taskID string, stats CompressionStats)
	OnArtifactPruning(taskID string, itemsRemoved int, tokensFreed int)
	OnBudgetExceeded(taskID string, attempted int, available int)
}

// CheckpointTelemetry extends telemetry with checkpoint lifecycle events.
type CheckpointTelemetry interface {
	OnCheckpointCreated(taskID string, checkpointID string, nodeID string)
	OnCheckpointRestored(taskID string, checkpointID string)
	OnGraphResume(taskID string, checkpointID string, nodeID string)
}
