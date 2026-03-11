package memory

import (
	"context"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

// WorkflowRunStatus captures the lifecycle of a workflow or run record.
type WorkflowRunStatus string

const (
	WorkflowRunStatusPending     WorkflowRunStatus = "pending"
	WorkflowRunStatusRunning     WorkflowRunStatus = "running"
	WorkflowRunStatusCompleted   WorkflowRunStatus = "completed"
	WorkflowRunStatusFailed      WorkflowRunStatus = "failed"
	WorkflowRunStatusCanceled    WorkflowRunStatus = "canceled"
	WorkflowRunStatusNeedsReplan WorkflowRunStatus = "needs_replan"
)

// StepStatus captures the current durable state of a plan step.
type StepStatus string

const (
	StepStatusPending     StepStatus = "pending"
	StepStatusRunning     StepStatus = "running"
	StepStatusCompleted   StepStatus = "completed"
	StepStatusFailed      StepStatus = "failed"
	StepStatusCanceled    StepStatus = "canceled"
	StepStatusInvalidated StepStatus = "invalidated"
	StepStatusNeedsReplan StepStatus = "needs_replan"
)

// KnowledgeKind distinguishes projected structured records.
type KnowledgeKind string

const (
	KnowledgeKindFact     KnowledgeKind = "fact"
	KnowledgeKindIssue    KnowledgeKind = "issue"
	KnowledgeKindDecision KnowledgeKind = "decision"
)

// ArtifactStorageKind indicates how raw data is persisted.
type ArtifactStorageKind string

const (
	ArtifactStorageInline ArtifactStorageKind = "inline"
	ArtifactStorageRef    ArtifactStorageKind = "ref"
)

// WorkflowRecord is the durable identity and cursor for a workflow.
type WorkflowRecord struct {
	WorkflowID   string
	TaskID       string
	TaskType     core.TaskType
	Instruction  string
	Status       WorkflowRunStatus
	CursorStepID string
	Version      int64
	Metadata     map[string]any
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// WorkflowRunRecord describes a single execution attempt for a workflow.
type WorkflowRunRecord struct {
	RunID          string
	WorkflowID     string
	Status         WorkflowRunStatus
	AgentName      string
	AgentMode      string
	RuntimeVersion string
	Metadata       map[string]any
	StartedAt      time.Time
	FinishedAt     *time.Time
}

// WorkflowPlanRecord stores the immutable plan payload for a workflow.
type WorkflowPlanRecord struct {
	PlanID     string
	WorkflowID string
	RunID      string
	Plan       core.Plan
	PlanHash   string
	IsActive   bool
	CreatedAt  time.Time
}

// WorkflowStepRecord stores one durable plan step and its current state.
type WorkflowStepRecord struct {
	WorkflowID   string
	PlanID       string
	StepID       string
	Ordinal      int
	Step         core.PlanStep
	Status       StepStatus
	Summary      string
	UpdatedAt    time.Time
	Dependencies []string
}

// StepRunRecord stores one execution attempt for a step.
type StepRunRecord struct {
	StepRunID      string
	WorkflowID     string
	RunID          string
	StepID         string
	Attempt        int
	Status         StepStatus
	Summary        string
	ResultData     map[string]any
	VerificationOK bool
	ErrorText      string
	StartedAt      time.Time
	FinishedAt     *time.Time
}

// StepArtifactRecord stores both prompt-oriented summary and raw payload/ref.
type StepArtifactRecord struct {
	ArtifactID        string
	WorkflowID        string
	StepRunID         string
	Kind              string
	ContentType       string
	StorageKind       ArtifactStorageKind
	SummaryText       string
	SummaryMetadata   map[string]any
	InlineRawText     string
	RawRef            string
	RawSizeBytes      int64
	CompressionMethod string
	CreatedAt         time.Time
}

// WorkflowArtifactRecord stores prompt-oriented summary plus raw payload/ref
// for workflow- or run-scoped artifacts such as planner outputs.
type WorkflowArtifactRecord struct {
	ArtifactID        string
	WorkflowID        string
	RunID             string
	Kind              string
	ContentType       string
	StorageKind       ArtifactStorageKind
	SummaryText       string
	SummaryMetadata   map[string]any
	InlineRawText     string
	RawRef            string
	RawSizeBytes      int64
	CompressionMethod string
	CreatedAt         time.Time
}

// WorkflowStageResultRecord stores one typed pipeline stage attempt for a workflow/run.
type WorkflowStageResultRecord struct {
	ResultID         string
	WorkflowID       string
	RunID            string
	StageName        string
	StageIndex       int
	ContractName     string
	ContractVersion  string
	PromptText       string
	ResponseJSON     string
	DecodedOutput    any
	ValidationOK     bool
	ErrorText        string
	RetryAttempt     int
	TransitionKind   string
	NextStage        string
	TransitionReason string
	StartedAt        time.Time
	FinishedAt       time.Time
}

// PipelineCheckpointRecord stores a resumable pipeline checkpoint including
// the serialized execution context needed to restart from the next stage.
type PipelineCheckpointRecord struct {
	CheckpointID string
	TaskID       string
	WorkflowID   string
	RunID        string
	StageName    string
	StageIndex   int
	ContextJSON  string
	ResultJSON   string
	CreatedAt    time.Time
}

// KnowledgeRecord stores extracted facts, issues, or decisions for prompting.
type KnowledgeRecord struct {
	RecordID   string
	WorkflowID string
	StepRunID  string
	StepID     string
	Kind       KnowledgeKind
	Title      string
	Content    string
	Status     string
	Metadata   map[string]any
	CreatedAt  time.Time
}

// WorkflowEventRecord stores an append-only workflow event.
type WorkflowEventRecord struct {
	EventID    string
	WorkflowID string
	RunID      string
	StepID     string
	EventType  string
	Message    string
	Metadata   map[string]any
	CreatedAt  time.Time
}

// WorkflowProviderSnapshotRecord stores provider-level runtime state captured for a workflow/run.
type WorkflowProviderSnapshotRecord struct {
	SnapshotID     string
	WorkflowID     string
	RunID          string
	ProviderID     string
	Recoverability core.RecoverabilityMode
	Descriptor     core.ProviderDescriptor
	Health         core.ProviderHealthSnapshot
	CapabilityIDs  []string
	TaskID         string
	Metadata       map[string]any
	State          any
	CapturedAt     time.Time
}

// WorkflowProviderSessionSnapshotRecord stores session-level runtime state captured for a workflow/run.
type WorkflowProviderSessionSnapshotRecord struct {
	SnapshotID string
	WorkflowID string
	RunID      string
	Session    core.ProviderSession
	Metadata   map[string]any
	State      any
	CapturedAt time.Time
}

// WorkflowDelegationRecord stores the durable state for one delegation.
type WorkflowDelegationRecord struct {
	DelegationID   string
	WorkflowID     string
	RunID          string
	TaskID         string
	State          core.DelegationState
	TrustClass     core.TrustClass
	Recoverability core.RecoverabilityMode
	Background     bool
	Request        core.DelegationRequest
	Result         *core.DelegationResult
	Metadata       map[string]any
	StartedAt      time.Time
	UpdatedAt      time.Time
}

// WorkflowDelegationTransitionRecord stores append-only lifecycle changes for a delegation.
type WorkflowDelegationTransitionRecord struct {
	TransitionID string
	DelegationID string
	WorkflowID   string
	RunID        string
	FromState    core.DelegationState
	ToState      core.DelegationState
	Metadata     map[string]any
	CreatedAt    time.Time
}

// InvalidationRecord captures downstream invalidation caused by reruns.
type InvalidationRecord struct {
	InvalidationID    string
	WorkflowID        string
	SourceStepID      string
	InvalidatedStepID string
	Reason            string
	CreatedAt         time.Time
}

// WorkflowStepSlice is the projected durable state for one executable step.
type WorkflowStepSlice struct {
	Workflow        WorkflowRecord
	Step            WorkflowStepRecord
	DependencySteps []WorkflowStepRecord
	DependencyRuns  []StepRunRecord
	Artifacts       []StepArtifactRecord
	Facts           []KnowledgeRecord
	Issues          []KnowledgeRecord
	Decisions       []KnowledgeRecord
	RecentEvents    []WorkflowEventRecord
}

// WorkflowStateStore is the durable source of truth for multi-step workflows.
type WorkflowStateStore interface {
	SchemaVersion(ctx context.Context) (int, error)

	CreateWorkflow(ctx context.Context, workflow WorkflowRecord) error
	GetWorkflow(ctx context.Context, workflowID string) (*WorkflowRecord, bool, error)
	ListWorkflows(ctx context.Context, limit int) ([]WorkflowRecord, error)
	UpdateWorkflowStatus(ctx context.Context, workflowID string, expectedVersion int64, status WorkflowRunStatus, cursorStepID string) (int64, error)

	CreateRun(ctx context.Context, run WorkflowRunRecord) error
	GetRun(ctx context.Context, runID string) (*WorkflowRunRecord, bool, error)
	UpdateRunStatus(ctx context.Context, runID string, status WorkflowRunStatus, finishedAt *time.Time) error

	SavePlan(ctx context.Context, plan WorkflowPlanRecord) error
	GetActivePlan(ctx context.Context, workflowID string) (*WorkflowPlanRecord, bool, error)
	ListSteps(ctx context.Context, workflowID string) ([]WorkflowStepRecord, error)
	ListReadySteps(ctx context.Context, workflowID string) ([]WorkflowStepRecord, error)
	UpdateStepStatus(ctx context.Context, workflowID, stepID string, status StepStatus, summary string) error
	InvalidateDependents(ctx context.Context, workflowID, sourceStepID, reason string) ([]InvalidationRecord, error)
	ListInvalidations(ctx context.Context, workflowID string) ([]InvalidationRecord, error)

	CreateStepRun(ctx context.Context, run StepRunRecord) error
	ListStepRuns(ctx context.Context, workflowID, stepID string) ([]StepRunRecord, error)

	UpsertArtifact(ctx context.Context, artifact StepArtifactRecord) error
	ListArtifacts(ctx context.Context, workflowID, stepRunID string) ([]StepArtifactRecord, error)
	UpsertWorkflowArtifact(ctx context.Context, artifact WorkflowArtifactRecord) error
	ListWorkflowArtifacts(ctx context.Context, workflowID, runID string) ([]WorkflowArtifactRecord, error)
	SaveStageResult(ctx context.Context, record WorkflowStageResultRecord) error
	ListStageResults(ctx context.Context, workflowID, runID string) ([]WorkflowStageResultRecord, error)
	GetLatestValidStageResult(ctx context.Context, workflowID, runID, stageName string) (*WorkflowStageResultRecord, bool, error)
	SavePipelineCheckpoint(ctx context.Context, record PipelineCheckpointRecord) error
	LoadPipelineCheckpoint(ctx context.Context, taskID, checkpointID string) (*PipelineCheckpointRecord, bool, error)
	ListPipelineCheckpoints(ctx context.Context, taskID string) ([]string, error)

	PutKnowledge(ctx context.Context, record KnowledgeRecord) error
	ListKnowledge(ctx context.Context, workflowID string, kind KnowledgeKind, unresolvedOnly bool) ([]KnowledgeRecord, error)

	AppendEvent(ctx context.Context, event WorkflowEventRecord) error
	ListEvents(ctx context.Context, workflowID string, limit int) ([]WorkflowEventRecord, error)

	ReplaceProviderSnapshots(ctx context.Context, workflowID, runID string, snapshots []WorkflowProviderSnapshotRecord) error
	ListProviderSnapshots(ctx context.Context, workflowID, runID string) ([]WorkflowProviderSnapshotRecord, error)
	ReplaceProviderSessionSnapshots(ctx context.Context, workflowID, runID string, snapshots []WorkflowProviderSessionSnapshotRecord) error
	ListProviderSessionSnapshots(ctx context.Context, workflowID, runID string) ([]WorkflowProviderSessionSnapshotRecord, error)
	UpsertDelegation(ctx context.Context, record WorkflowDelegationRecord) error
	ListDelegations(ctx context.Context, workflowID, runID string) ([]WorkflowDelegationRecord, error)
	AppendDelegationTransition(ctx context.Context, record WorkflowDelegationTransitionRecord) error
	ListDelegationTransitions(ctx context.Context, delegationID string) ([]WorkflowDelegationTransitionRecord, error)

	LoadStepSlice(ctx context.Context, workflowID, stepID string, eventLimit int) (*WorkflowStepSlice, bool, error)
}
