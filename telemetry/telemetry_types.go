package telemetry

import "time"

// EventType categorizes telemetry events.
type EventType string

const (
	EventGraphStart            EventType = "graph_start"
	EventGraphFinish           EventType = "graph_finish"
	EventGraphBranchMerged     EventType = "graph.branch_merged"
	EventNodeStart             EventType = "node_start"
	EventNodeFinish            EventType = "node_finish"
	EventNodeError             EventType = "node_error"
	EventAgentStart            EventType = "agent_start"
	EventAgentFinish           EventType = "agent_finish"
	EventLLMPrompt             EventType = "llm_prompt"
	EventLLMResponse           EventType = "llm_response"
	EventDelegationStart       EventType = "delegation_start"
	EventDelegationFinish      EventType = "delegation_finish"
	EventDelegationCancel      EventType = "delegation_cancel"
	EventCapabilityCall        EventType = "capability_call"
	EventCapabilityResult      EventType = "capability_result"
	EventToolCall              EventType = "tool_call"
	EventToolResult            EventType = "tool_result"
	EventStateChange           EventType = "state_change"
	EventInferenceError        EventType = "inference_error"
	EventInferenceTimeout      EventType = "inference_timeout"
	EventInferenceAbort        EventType = "inference_abort"
	EventBackendStateChange    EventType = "backend_state_change"
	EventBackendWarm           EventType = "backend_warm"
	EventBackendClose          EventType = "backend_close"
	EventBackendRestart        EventType = "backend_restart"
	EventChunkCommitted        EventType = "chunk.committed"
	EventSummaryCommitted      EventType = "summary.committed"
	EventContextPolicyReloaded EventType = "context_policy_reloaded"
	EventProviderSessionEnded  EventType = "provider_session_ended"
	EventCompilerWarning       EventType = "compiler_warning"
	EventBootstrapComplete     EventType = "bootstrap_complete"
)

// Domain event types bridged in Phase 6 (silent domain bridging). These carry
// the same dot-qualified spelling as the decision-forensics events so the JSONL
// namespace is consistent: compiler.*, scheduler.*, chunk.*, sandbox.*.
const (
	EventCompilerStarted              EventType = "compiler.started"
	EventCompilerCompleted            EventType = "compiler.completed"
	EventCompilerCacheHit             EventType = "compiler.cache_hit"
	EventCompilerCacheMiss            EventType = "compiler.cache_miss"
	EventCompilerBudgetExceeded       EventType = "compiler.budget_exceeded"
	EventCompilerSummarySubstituted   EventType = "compiler.summary_substituted"
	EventChunkStaled                  EventType = "chunk.staled"
	EventChunkInvalidated             EventType = "chunk.invalidated"
	EventSchedulerJobStarted          EventType = "scheduler.job_started"
	EventSchedulerJobCompleted        EventType = "scheduler.job_completed"
	EventSchedulerJobFailed           EventType = "scheduler.job_failed"
	EventSchedulerJobSkipped          EventType = "scheduler.job_skipped"
	EventSandboxCommandDenied         EventType = "sandbox.command_denied"
	EventSandboxCommandExecuted       EventType = "sandbox.command_executed"
	EventSandboxFailure               EventType = "sandbox.failure"
	EventSandboxOrphanReaped          EventType = "sandbox.orphan_reaped"
	EventSandboxOutputCeilingExceeded EventType = "sandbox.output_ceiling_exceeded"
	EventSandboxProtectedPathEscaped  EventType = "sandbox.protected_path_escaped"
	EventSandboxImagePinned           EventType = "sandbox.image_pinned"
	EventSandboxImageUnpinned         EventType = "sandbox.image_unpinned"
	EventBootDegraded                 EventType = "boot.degraded"
	// EventTaskRejected reports a task submission refused because the runtime
	// is degraded (D7). It carries the boot failure reason in metadata and is
	// the only record a rejected task leaves behind: the guard in executeTask
	// runs before envelope assembly and lifecycle bookkeeping.
	EventTaskRejected EventType = "task.rejected"
)

// Paradigm lifecycle events emitted by the cognitionzoo paradigms (HTN,
// reflection, planner) through the standard telemetry.Telemetry +
// StampCorrelation path (spec §1.7). They use the same dot-qualified spelling
// as the scheduler/compiler domain events so the JSONL namespace stays
// consistent.
const (
	EventHTNPlanStarted        EventType = "htn.plan.started"
	EventHTNPlanFailed         EventType = "htn.plan.failed"
	EventHTNStepStarted        EventType = "htn.step.started"
	EventHTNStepCompleted      EventType = "htn.step.completed"
	EventHTNExecutionCompleted EventType = "htn.execution.completed"

	EventReflectionIteration EventType = "reflection.iteration"
	EventReflectionCompleted EventType = "reflection.completed"

	EventPlannerPlanStarted   EventType = "planner.plan.started"
	EventPlannerPlanCompleted EventType = "planner.plan.completed"
	EventPlannerPlanFailed    EventType = "planner.plan.failed"
)

const (
	EventBudgetSnapshot       = "budget.snapshot"
	EventSessionResetRequired = "session.reset_required"
)

// Rollback token lifecycle events emitted by the capability registry
// (SBH-1 D-9). The stored event carries the token ID and the tool name only —
// never the raw invocation args those tokens reference.
const (
	EventRollbackTokenStored  EventType = "rollback.token_stored"
	EventRollbackTokenExpired EventType = "rollback.token_expired"
)

// Event captures structured telemetry data.
type Event struct {
	Type      EventType      `json:"type"`
	SessionID string         `json:"session_id,omitempty"`
	RunID     string         `json:"run_id,omitempty"`
	TraceID   string         `json:"trace_id,omitempty"`
	AgentID   string         `json:"agent_id,omitempty"`
	NodeID    string         `json:"node_id,omitempty"`
	SpanID    string         `json:"span_id,omitempty"`
	TaskID    string         `json:"task_id,omitempty"`
	Message   string         `json:"message,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Seq       uint64         `json:"seq,omitempty"`
	Partition string         `json:"partition,omitempty"`
	Payload   []byte         `json:"payload,omitempty"`
	Actor     string         `json:"actor,omitempty"`
}

// Telemetry captures execution traces emitted by the graph runtime.
type Telemetry interface {
	Emit(event Event)
}

// BudgetTelemetry extends telemetry with budget-management signals.
type BudgetTelemetry interface {
	OnArtifactPruning(taskID string, itemsRemoved int, tokensFreed int)
	OnBudgetExceeded(taskID string, attempted int, available int)
}

// CheckpointTelemetry extends telemetry with checkpoint lifecycle events.
type CheckpointTelemetry interface {
	OnCheckpointCreated(taskID string, checkpointID string, nodeID string)
	OnCheckpointRestored(taskID string, checkpointID string)
	OnGraphResume(taskID string, checkpointID string, nodeID string)
}
