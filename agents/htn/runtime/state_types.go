package runtime

import (
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
)

const (
	contextKeyTask                     = "htn.task"
	contextKeyTaskType                 = "htn.task_type"
	contextKeySelectedMethod           = "htn.selected_method"
	contextKeyPlan                     = "htn.plan"
	contextKeyExecution                = "htn.execution"
	contextKeyCompletedSteps           = "htn.execution.completed_steps"
	contextKeyState                    = "htn.state"
	contextKeyStateError               = "htn.state_error"
	contextKeyMetrics                  = "htn.metrics"
	contextKeyTermination              = "htn.termination"
	contextKeyWorkflowRetrieval        = "htn.workflow_retrieval"
	contextKeyWorkflowRetrievalPayload = "htn.workflow_retrieval_payload"
	contextKeyRetrievalApplied         = "htn.retrieval_applied"
	contextKeyResumeCheckpointID       = "htn.resume_checkpoint_id"
	contextKeyPreflightReport          = "htn.preflight.report"
	contextKeyPreflightError           = "htn.preflight.error"
	contextKeyCheckpoint               = "htn.checkpoint"
	contextKeyCheckpointRef            = "htn.checkpoint_ref"
	contextKeyCheckpointSummary        = "htn.checkpoint_summary"
	contextKeyRunSummaryRef            = "htn.run_summary_ref"
	contextKeyRunSummarySummary        = "htn.run_summary_summary"
	contextKeyExecutionMetricsRef      = "htn.execution_metrics_ref"
	contextKeyExecutionMetricsSummary  = "htn.execution_metrics_summary"
	contextKeyLastRecoveryNotes        = "htn.last_recovery_notes"
	contextKeyLastRecoveryDiag         = "htn.last_recovery_diagnosis"
	contextKeyLastFailureStep          = "htn.last_failed_step"
	contextKeyLastFailureError         = "htn.last_failure_error"
	contextKnowledgeSummary            = "htn.summary"
	contextKnowledgeTaskType           = "htn.task_type"
	contextKnowledgeMethod             = "htn.selected_method"
	contextKnowledgeTermination        = "htn.termination"
	legacyPlanCompletedStepsKey        = "plan.completed_steps"
	htnSchemaVersion                   = 1
)

// TaskState summarizes the active HTN task in a durable, serializable form.
type TaskState struct {
	ID          string            `json:"id"`
	Type        core.TaskType     `json:"type"`
	Instruction string            `json:"instruction"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// MethodState summarizes the selected decomposition method without retaining
// executable function fields.
type MethodState struct {
	Name                 string                    `json:"name"`
	TaskType             core.TaskType             `json:"task_type"`
	Priority             int                       `json:"priority"`
	SubtaskCount         int                       `json:"subtask_count"`
	OperatorCount        int                       `json:"operator_count"`
	RequiredCapabilities []core.CapabilitySelector `json:"required_capabilities,omitempty"`
}

// ExecutionState tracks HTN runtime progress separately from the plan.
type ExecutionState struct {
	WorkflowID         string   `json:"workflow_id,omitempty"`
	RunID              string   `json:"run_id,omitempty"`
	CompletedSteps     []string `json:"completed_steps,omitempty"`
	LastCompletedStep  string   `json:"last_completed_step,omitempty"`
	PlannedStepCount   int      `json:"planned_step_count"`
	CompletedStepCount int      `json:"completed_step_count"`
	Resumed            bool     `json:"resumed"`
	ResumeCheckpointID string   `json:"resume_checkpoint_id,omitempty"`
}

// Metrics captures simple HTN execution counters for inspection/debugging.
type Metrics struct {
	PlannedStepCount   int `json:"planned_step_count"`
	CompletedStepCount int `json:"completed_step_count"`
}

// PreflightState captures the last explicit runtime-graph preflight result
// observed by HTN before execution.
type PreflightState struct {
	Report *graph.PreflightReport `json:"report,omitempty"`
	Error  string                 `json:"error,omitempty"`
}

// CheckpointState captures the HTN-owned checkpoint payload persisted inside
// the broader workflow checkpoint context.
type CheckpointState struct {
	SchemaVersion  int       `json:"schema_version"`
	CheckpointID   string    `json:"checkpoint_id,omitempty"`
	StageName      string    `json:"stage_name,omitempty"`
	StageIndex     int       `json:"stage_index,omitempty"`
	WorkflowID     string    `json:"workflow_id,omitempty"`
	RunID          string    `json:"run_id,omitempty"`
	CompletedSteps []string  `json:"completed_steps,omitempty"`
	Snapshot       *HTNState `json:"snapshot,omitempty"`
}

// HTNState is the canonical typed snapshot of HTN runtime state persisted in
// core.Context.
type HTNState struct {
	SchemaVersion      int            `json:"schema_version"`
	Task               TaskState      `json:"task"`
	Method             MethodState    `json:"method"`
	Plan               *core.Plan     `json:"plan,omitempty"`
	Execution          ExecutionState `json:"execution"`
	Metrics            Metrics        `json:"metrics"`
	Preflight          PreflightState `json:"preflight,omitempty"`
	Termination        string         `json:"termination,omitempty"`
	RetrievalApplied   bool           `json:"retrieval_applied"`
	ResumeCheckpointID string         `json:"resume_checkpoint_id,omitempty"`
}
