package memory

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// WorkflowStateStore defines the interface for workflow state persistence.
// This interface is used by the authorization package for delegation persistence.
type WorkflowStateStore interface {
	// GetDelegation retrieves a delegation entry by ID.
	GetDelegation(ctx context.Context, delegationID string) (*DelegationEntry, error)
	// UpsertDelegation creates or updates a delegation entry.
	UpsertDelegation(ctx context.Context, entry DelegationEntry) error
	// AppendDelegationTransition records a state transition for a delegation.
	AppendDelegationTransition(ctx context.Context, transition DelegationTransitionEntry) error
	// GetWorkflowArtifact retrieves an artifact by ID.
	GetWorkflowArtifact(ctx context.Context, artifactID string) (*WorkflowArtifactRecord, error)
	// UpsertWorkflowArtifact creates or updates a workflow artifact.
	UpsertWorkflowArtifact(ctx context.Context, artifact WorkflowArtifactRecord) error
	// CreateWorkflow creates a new workflow entry.
	CreateWorkflow(ctx context.Context, workflowID string) error
	// ListWorkflows lists all workflows.
	ListWorkflows(ctx context.Context) ([]string, error)
	// Close closes the store.
	Close() error
}

// WorkflowRecord represents a workflow entity (legacy type for non-archaeo code).
type WorkflowRecord struct {
	WorkflowID  string
	TaskID      string
	TaskType    string
	Instruction string
	Status      WorkflowRunStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Metadata    map[string]any
}

// WorkflowRunRecord represents a single execution run of a workflow (legacy type for non-archaeo code).
type WorkflowRunRecord struct {
	RunID          string
	WorkflowID     string
	Status         WorkflowRunStatus
	AgentMode      string
	RuntimeVersion string
	AgentName      string
	StartedAt      time.Time
	CompletedAt    *time.Time
	FinishedAt     *time.Time
	Metadata       map[string]any
}

// WorkflowRunStatus represents the status of a workflow run.
type WorkflowRunStatus string

const (
	// WorkflowRunStatusRunning indicates the workflow is running.
	WorkflowRunStatusRunning WorkflowRunStatus = "running"
	// WorkflowRunStatusCompleted indicates the workflow completed successfully.
	WorkflowRunStatusCompleted WorkflowRunStatus = "completed"
	// WorkflowRunStatusFailed indicates the workflow failed.
	WorkflowRunStatusFailed WorkflowRunStatus = "failed"
	// WorkflowRunStatusCanceled indicates the workflow was canceled.
	WorkflowRunStatusCanceled WorkflowRunStatus = "canceled"
	// WorkflowRunStatusNeedsReplan indicates the workflow needs replanning.
	WorkflowRunStatusNeedsReplan WorkflowRunStatus = "needs_replan"
)

// DelegationEntry represents a persisted delegation state.
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

// WorkflowProjectionService provides resource projection for workflow delegations.
type WorkflowProjectionService struct {
	Store WorkflowStateStore
}

// Project projects a workflow resource based on the provided URI.
func (s *WorkflowProjectionService) Project(ctx context.Context, uri WorkflowResourceURI) (*core.ResourceReadResult, error) {
	// Implementation would query the store and return the projected resource.
	// This is a simplified implementation.
	if s.Store == nil {
		return nil, nil
	}
	return &core.ResourceReadResult{
		Contents: []core.ContentBlock{},
		Metadata: map[string]any{
			"uri": uri.String(),
		},
	}, nil
}

// WorkflowResourceURI represents a parsed workflow resource URI.
type WorkflowResourceURI struct {
	WorkflowID string
	RunID      string
	StepID     string
	Role       WorkflowProjectionRole
	ResourceID string
}

// String returns the string representation of the URI.
func (u WorkflowResourceURI) String() string {
	// Simplified string representation
	return u.WorkflowID + "/" + u.RunID + "/" + u.ResourceID
}

// ParseWorkflowResourceURI parses a workflow resource URI string.
func ParseWorkflowResourceURI(uri string) (WorkflowResourceURI, error) {
	// Simplified parser - in production this would be more robust
	parts := splitURI(uri)
	if len(parts) < 2 {
		return WorkflowResourceURI{}, nil
	}
	return WorkflowResourceURI{
		WorkflowID: parts[0],
		RunID:      parts[1],
		ResourceID: parts[len(parts)-1],
	}, nil
}

// DefaultWorkflowProjectionRefs generates default resource references for a workflow.
func DefaultWorkflowProjectionRefs(workflowID, runID, stepID string, role WorkflowProjectionRole) []string {
	// Return a list of default resource references for the given workflow context.
	return []string{
		workflowID + "/" + runID + "/context",
		workflowID + "/" + runID + "/state",
	}
}

// splitURI is a helper function to split a URI into parts.
func splitURI(uri string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(uri); i++ {
		if uri[i] == '/' {
			if i > start {
				parts = append(parts, uri[start:i])
			}
			start = i + 1
		}
	}
	if start < len(uri) {
		parts = append(parts, uri[start:])
	}
	return parts
}
