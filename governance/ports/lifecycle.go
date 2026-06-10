package ports

import (
	"context"
	"time"
)

// DelegationRepository is the governance-owned interface for persisting
// delegation lifecycle state. execution/agentlifecycle implements this.
type DelegationRepository interface {
	UpsertDelegation(ctx context.Context, entry DelegationEntry) error
	GetDelegation(ctx context.Context, delegationID string) (*DelegationEntry, error)
	ListDelegations(ctx context.Context, workflowID string) ([]DelegationEntry, error)
	ListDelegationsByRun(ctx context.Context, runID string) ([]DelegationEntry, error)
	AppendDelegationTransition(ctx context.Context, transition DelegationTransitionEntry) error
	ListDelegationTransitions(ctx context.Context, delegationID string) ([]DelegationTransitionEntry, error)
	UpsertArtifact(ctx context.Context, artifact WorkflowArtifactRecord) error
}

// DelegationEntry is the governance-owned view of a delegation record.
type DelegationEntry struct {
	DelegationID   string
	WorkflowID     string
	RunID          string
	TaskID         string
	State          string
	TrustClass     string
	Recoverability string
	Background     bool
	Request        any
	Result         any
	Metadata       map[string]any
	StartedAt      time.Time
	UpdatedAt      time.Time
}

// DelegationTransitionEntry records a state transition in a delegation.
type DelegationTransitionEntry struct {
	TransitionID string
	DelegationID string
	WorkflowID   string
	RunID        string
	ToState      string
	Metadata     map[string]any
	CreatedAt    time.Time
}

// WorkflowArtifactRecord is the governance-owned view of a workflow artifact.
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
	ArtifactStorageInline   ArtifactStorageKind = "inline"
	ArtifactStorageExternal ArtifactStorageKind = "external"
)

// LifecycleView is the governance-owned interface for persisting
// delegation lifecycle data. execution/agentlifecycle implements it.
type LifecycleView interface {
	PersistDelegation(entry DelegationEntry) error
	PersistDelegationTransition(transition DelegationTransitionEntry) error
	StoreArtifact(record WorkflowArtifactRecord) error
}
