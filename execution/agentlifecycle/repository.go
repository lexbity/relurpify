package agentlifecycle

import (
	"context"
)

// Repository defines the lifecycle persistence adapter interface.
// This is the narrow interface that agentlifecycle depends on for persistence.
// Implementations live in framework/persistence and framework/graphdb.
type Repository interface {
	// Workflow operations
	CreateWorkflow(ctx context.Context, workflow WorkflowRecord) error
	GetWorkflow(ctx context.Context, workflowID string) (*WorkflowRecord, error)
	ListWorkflows(ctx context.Context) ([]WorkflowRecord, error)

	// Run operations
	CreateRun(ctx context.Context, run WorkflowRunRecord) error
	GetRun(ctx context.Context, runID string) (*WorkflowRunRecord, error)
	ListRuns(ctx context.Context, workflowID string) ([]WorkflowRunRecord, error)
	UpdateRunStatus(ctx context.Context, runID string, status string) error

	// Delegation operations
	UpsertDelegation(ctx context.Context, entry DelegationEntry) error
	GetDelegation(ctx context.Context, delegationID string) (*DelegationEntry, error)
	ListDelegations(ctx context.Context, workflowID string) ([]DelegationEntry, error)
	ListDelegationsByRun(ctx context.Context, runID string) ([]DelegationEntry, error)
	AppendDelegationTransition(ctx context.Context, transition DelegationTransitionEntry) error
	ListDelegationTransitions(ctx context.Context, delegationID string) ([]DelegationTransitionEntry, error)

	// Event operations
	AppendEvent(ctx context.Context, event WorkflowEventRecord) error
	ListEvents(ctx context.Context, workflowID string, limit int) ([]WorkflowEventRecord, error)
	ListEventsByRun(ctx context.Context, runID string, limit int) ([]WorkflowEventRecord, error)

	// Artifact operations
	UpsertArtifact(ctx context.Context, artifact WorkflowArtifactRecord) error
	GetArtifact(ctx context.Context, artifactID string) (*WorkflowArtifactRecord, error)
	ListArtifacts(ctx context.Context, workflowID string) ([]WorkflowArtifactRecord, error)
	ListArtifactsByRun(ctx context.Context, runID string) ([]WorkflowArtifactRecord, error)

	// Lineage binding operations
	UpsertLineageBinding(ctx context.Context, binding LineageBindingRecord) error
	GetLineageBinding(ctx context.Context, bindingID string) (*LineageBindingRecord, error)
	FindLineageBindingByWorkflow(ctx context.Context, workflowID string) ([]LineageBindingRecord, error)
	FindLineageBindingByRun(ctx context.Context, runID string) ([]LineageBindingRecord, error)
	FindLineageBindingByLineageID(ctx context.Context, lineageID string) (*LineageBindingRecord, error)
	FindLineageBindingByAttemptID(ctx context.Context, attemptID string) (*LineageBindingRecord, error)

	// Close
	Close() error
}
