package agentlifecycle

import (
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// WorkflowRecord represents a workflow lifecycle entity.
type WorkflowRecord struct {
	WorkflowID   string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Metadata     map[string]any
}

// WorkflowRunRecord represents a single execution run of a workflow.
type WorkflowRunRecord struct {
	RunID        string
	WorkflowID   string
	Status       string
	StartedAt    time.Time
	CompletedAt  *time.Time
	Metadata     map[string]any
}

// DelegationEntry represents a persisted delegation state.
// This is moved from framework/memory to agentlifecycle as it is lifecycle state.
type DelegationEntry struct {
	DelegationID   string
	WorkflowID     string
	RunID          string
	TaskID         string
	State          string
	TrustClass     string
	Recoverability string
	Background     bool
	Request        core.DelegationRequest
	Result         *core.DelegationResult
	Metadata       map[string]any
	StartedAt      time.Time
	UpdatedAt      time.Time
}

// DelegationTransitionEntry records a state transition for a delegation.
type DelegationTransitionEntry struct {
	TransitionID string
	DelegationID string
	WorkflowID   string
	RunID        string
	ToState      string
	Metadata     map[string]any
	CreatedAt    time.Time
}

// WorkflowEventRecord represents a runtime workflow event.
type WorkflowEventRecord struct {
	EventID      string
	WorkflowID   string
	RunID        string
	EventType    string
	Payload      map[string]any
	Sequence     uint64
	CreatedAt    time.Time
}

// WorkflowArtifactRecord represents a persisted workflow artifact.
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
	RawSizeBytes      int64
	CompressionMethod string
	CreatedAt         time.Time
}

// ArtifactStorageKind indicates how an artifact is stored.
type ArtifactStorageKind string

const (
	// ArtifactStorageInline means the artifact content is stored inline.
	ArtifactStorageInline ArtifactStorageKind = "inline"
	// ArtifactStorageExternal means the artifact is stored externally (e.g., S3, file).
	ArtifactStorageExternal ArtifactStorageKind = "external"
)

// LineageBindingRecord represents a lineage binding for runtime bridges.
type LineageBindingRecord struct {
	BindingID    string
	WorkflowID   string
	RunID        string
	LineageID    string
	AttemptID    string
	BindingType  string
	Metadata     map[string]any
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// WorkflowProjectionRole represents the role of a workflow projection.
type WorkflowProjectionRole string

const (
	// WorkflowProjectionRoleArchitect is the architect role.
	WorkflowProjectionRoleArchitect WorkflowProjectionRole = "architect"
	// WorkflowProjectionRoleReviewer is the reviewer role.
	WorkflowProjectionRoleReviewer WorkflowProjectionRole = "reviewer"
	// WorkflowProjectionRoleVerifier is the verifier role.
	WorkflowProjectionRoleVerifier WorkflowProjectionRole = "verifier"
	// WorkflowProjectionRoleExecutor is the executor role.
	WorkflowProjectionRoleExecutor WorkflowProjectionRole = "executor"
)
