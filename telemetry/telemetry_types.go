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
	EventTombstonePreserved           EventType = "knowledge.tombstone_preserved"
	EventEventDropped                 EventType = "knowledge.event_dropped"
	EventInvalidationDegraded         EventType = "knowledge.invalidation_degraded"
	EventCaptureGrounded              EventType = "capture.grounded"
	EventCaptureQuarantined           EventType = "capture.quarantined"
	EventCaptureEpistemicsDowngraded  EventType = "capture.epistemics_downgraded"
	EventCaptureGroundFailed          EventType = "capture.ground_failed"
	EventCaptureSinkAbsent            EventType = "capture.sink_absent"
	EventEpochClosed                  EventType = "epoch.closed"
	EventStreamAbandoned              EventType = "contextstream.stream_abandoned"
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
	EventSandboxProtectedPathSelf     EventType = "sandbox.protected_path_self"
	EventSandboxImagePinned           EventType = "sandbox.image_pinned"
	EventSandboxImageUnpinned         EventType = "sandbox.image_unpinned"
	EventBootDegraded                 EventType = "boot.degraded"
	// EventTaskRejected reports a task submission refused because the runtime
	// is degraded (D7). It carries the boot failure reason in metadata and is
	// the only record a rejected task leaves behind: the guard in executeTask
	// runs before envelope assembly and lifecycle bookkeeping.
	EventTaskRejected EventType = "task.rejected"
)

// Runtime-lifecycle events emitted by the relurpish runtime's quiesce path
// (Phase 7): shutdown accounting surfaces what Close waited for, cancelled,
// and abandoned so "the store closed under an active writer" is observable
// instead of silent.
const (
	// EventShutdownDrain reports one Runtime.Close quiesce outcome.
	EventShutdownDrain EventType = "runtime.shutdown_drain"
	// EventShutdownAbandoned names a run that outlived its shutdown wait.
	EventShutdownAbandoned EventType = "runtime.shutdown_abandoned"
	// EventCheckpointMirrorFailed reports a durable checkpoint mirror write
	// failure: a mirror error is never reported as a successful checkpoint.
	EventCheckpointMirrorFailed EventType = "checkpoint.mirror_failed"
)

// EventTapeRecordFailed: tape recorder write failure, absorbed from the
// dissolved platform/observability vocabulary (S4). The other observability
// consts collided with existing telemetry consts, which won per the
// reconciliation rule (event_reconciliation_test.go).
const (
	EventTapeRecordFailed EventType = "tape.record_failed"
)

// Durable-jobs lifecycle events (S8, §5.8). The runner stamps every state
// transition with one of these — no silent transitions (NFR-9).
const (
	EventJobSubmitted        EventType = "job.submitted"
	EventJobClaimed          EventType = "job.claimed"
	EventJobCompleted        EventType = "job.completed"
	EventJobFailed           EventType = "job.failed"
	EventJobRetried          EventType = "job.retried"
	EventJobInterrupted      EventType = "job.interrupted"
	EventJobDuplicateIgnored EventType = "job.duplicate_ignored"
	EventSpoolMalformed      EventType = "spool.malformed"
	EventRunnerStarted       EventType = "runner.started"
	EventRunnerStopped       EventType = "runner.stopped"
	EventRunnerLockContented EventType = "runner.lock_contented"
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
	// EventReflectionReviewed frames one directive-mode review verdict.
	EventReflectionReviewed EventType = "reflection.reviewed"
	// EventReflectionRevisionCapped marks the authored revise loop hitting its
	// revision bound; the loop ends normally with the last verdict standing.
	EventReflectionRevisionCapped EventType = "reflection.revision_capped"

	EventPlannerPlanStarted   EventType = "planner.plan.started"
	EventPlannerPlanCompleted EventType = "planner.plan.completed"
	EventPlannerPlanFailed    EventType = "planner.plan.failed"
	// EventPlannerPhase frames each directive-mode phase boundary
	// (plan/execute/verify/summarize) with its provenance origin.
	EventPlannerPhase EventType = "planner.phase"
)

// Operational-failure and grounded-restore events (Wave 2 Phase 4). The
// failure protocol classifies every runtime step failure into the §3.5
// taxonomy; step.fallback_activated marks an authored fallback firing; and the
// grounding.* events surface the Wave 1 port composition state honestly at
// boot and at dispatch-time restore.
const (
	// EventStepOperationalFailure is emitted for every classified operational
	// step failure. Metadata: step_id, paradigm, kind, on_error, action_taken.
	EventStepOperationalFailure EventType = "step.operational_failure"
	// EventStepFallbackActivated marks an authored fallback agent taking over a
	// failed step.
	EventStepFallbackActivated EventType = "step.fallback_activated"
	// EventGroundingPortsUnwired is the single info-level boot event emitted
	// when no StateReground source is composed (declared cold-start mode).
	EventGroundingPortsUnwired EventType = "grounding.ports_unwired"
	// EventGroundingRegroundFailed reports a dispatch-time reground query that
	// failed; the run continues cold rather than silently restoring nothing.
	EventGroundingRegroundFailed EventType = "grounding.reground_failed"
	// EventFrameExpiredLateAnswer is emitted when a resolution arrives for an
	// interaction frame whose deadline has already passed. Nothing changes:
	// the stale-consent hole is closed at the resolver boundary (D12/FR-18).
	EventFrameExpiredLateAnswer EventType = "frame.expired_late_answer"
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
	// ActorKind classifies the actor (e.g. "agent", "system"). The
	// observability.Event Actor struct was flattened into Actor (the ID)
	// plus this field when the two Event structs unified (S4).
	ActorKind string `json:"actor_kind,omitempty"`
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
