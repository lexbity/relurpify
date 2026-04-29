// Package store provides workflow state persistence for the rex package.
package store

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/framework/memory"
)

// Re-export framework memory types for rex store compatibility.
type (
	// WorkflowRunStatus is an alias to framework memory type.
	WorkflowRunStatus = memory.WorkflowRunStatus
	// WorkflowRecord is an alias to framework memory type.
	WorkflowRecord = memory.WorkflowRecord
	// WorkflowRunRecord is an alias to framework memory type.
	WorkflowRunRecord = memory.WorkflowRunRecord
	// WorkflowArtifactRecord is an alias to framework memory type.
	WorkflowArtifactRecord = memory.WorkflowArtifactRecord
)

// WorkflowEventRecord stores workflow events.
type WorkflowEventRecord struct {
	EventID    string
	WorkflowID string
	RunID      string
	EventType  string
	Message    string
	Metadata   map[string]any
	CreatedAt  time.Time
}

// LineageBindingRecord stores workflow-to-lineage bindings.
type LineageBindingRecord struct {
	WorkflowID string
	RunID      string
	LineageID  string
	AttemptID  string
	RuntimeID  string
	SessionID  string
	State      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// LineageBindingStore persists workflow-to-lineage bindings.
type LineageBindingStore interface {
	UpsertLineageBinding(ctx context.Context, record LineageBindingRecord) error
	FindLineageBindingsByLineageID(ctx context.Context, lineageID string) ([]LineageBindingRecord, error)
	FindLineageBindingsByAttemptID(ctx context.Context, attemptID string) ([]LineageBindingRecord, error)
}

// WorkflowStore persists workflow state.
type WorkflowStore interface {
	GetWorkflow(ctx context.Context, workflowID string) (WorkflowRecord, bool, error)
	CreateWorkflow(ctx context.Context, record WorkflowRecord) error
	GetRun(ctx context.Context, runID string) (WorkflowRunRecord, bool, error)
	CreateRun(ctx context.Context, record WorkflowRunRecord) error
	GetLineageBinding(ctx context.Context, workflowID, runID string) (LineageBindingRecord, bool, error)
	ListWorkflowArtifacts(ctx context.Context, workflowID, runID string) ([]WorkflowArtifactRecord, error)
	ListEvents(ctx context.Context, workflowID string, limit int) ([]WorkflowEventRecord, error)
	AppendEvent(ctx context.Context, record WorkflowEventRecord) error
	UpsertWorkflowArtifact(ctx context.Context, record WorkflowArtifactRecord) error
	UpdateRunStatus(ctx context.Context, runID string, status WorkflowRunStatus, finishedAt *time.Time) error
	UpdateWorkflowStatus(ctx context.Context, workflowID string, version int, status WorkflowRunStatus, reason string) (bool, error)
}
