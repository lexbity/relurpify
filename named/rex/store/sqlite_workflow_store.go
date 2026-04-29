// Package store provides workflow state persistence for the rex package.
package store

import (
	"context"
	"errors"
	"time"
)

// SQLiteWorkflowStore is a SQLite-backed implementation of WorkflowStore.
// This is a stub implementation for compilation; full persistence to be implemented.
type SQLiteWorkflowStore struct {
	// Stub - actual implementation requires database integration
}

// GetWorkflow retrieves a workflow by ID.
func (s *SQLiteWorkflowStore) GetWorkflow(ctx context.Context, workflowID string) (WorkflowRecord, bool, error) {
	return WorkflowRecord{}, false, nil
}

// CreateWorkflow creates a new workflow record.
func (s *SQLiteWorkflowStore) CreateWorkflow(ctx context.Context, record WorkflowRecord) error {
	return nil
}

// GetRun retrieves a workflow run by ID.
func (s *SQLiteWorkflowStore) GetRun(ctx context.Context, runID string) (WorkflowRunRecord, bool, error) {
	return WorkflowRunRecord{}, false, nil
}

// CreateRun creates a new workflow run record.
func (s *SQLiteWorkflowStore) CreateRun(ctx context.Context, record WorkflowRunRecord) error {
	return nil
}

// GetLineageBinding retrieves a lineage binding by workflow and run ID.
func (s *SQLiteWorkflowStore) GetLineageBinding(ctx context.Context, workflowID, runID string) (LineageBindingRecord, bool, error) {
	return LineageBindingRecord{}, false, nil
}

// ListWorkflowArtifacts lists artifacts for a workflow run.
func (s *SQLiteWorkflowStore) ListWorkflowArtifacts(ctx context.Context, workflowID, runID string) ([]WorkflowArtifactRecord, error) {
	return nil, nil
}

// ListEvents lists events for a workflow.
func (s *SQLiteWorkflowStore) ListEvents(ctx context.Context, workflowID string, limit int) ([]WorkflowEventRecord, error) {
	return nil, nil
}

// AppendEvent appends an event to the workflow log.
func (s *SQLiteWorkflowStore) AppendEvent(ctx context.Context, record WorkflowEventRecord) error {
	return nil
}

// UpsertWorkflowArtifact stores or updates a workflow artifact.
func (s *SQLiteWorkflowStore) UpsertWorkflowArtifact(ctx context.Context, record WorkflowArtifactRecord) error {
	return nil
}

// UpdateRunStatus updates the status of a workflow run.
func (s *SQLiteWorkflowStore) UpdateRunStatus(ctx context.Context, runID string, status WorkflowRunStatus, finishedAt *time.Time) error {
	return nil
}

// UpdateWorkflowStatus updates the status of a workflow.
func (s *SQLiteWorkflowStore) UpdateWorkflowStatus(ctx context.Context, workflowID string, version int, status WorkflowRunStatus, reason string) (bool, error) {
	return true, nil
}

// LineageBindingStore returns a LineageBindingStore backed by the same SQLite database.
func (s *SQLiteWorkflowStore) LineageBindingStore() LineageBindingStore {
	return s
}

// UpsertLineageBinding creates or updates a lineage binding.
func (s *SQLiteWorkflowStore) UpsertLineageBinding(ctx context.Context, record LineageBindingRecord) error {
	return nil
}

// FindLineageBindingsByLineageID finds bindings by lineage ID.
func (s *SQLiteWorkflowStore) FindLineageBindingsByLineageID(ctx context.Context, lineageID string) ([]LineageBindingRecord, error) {
	return nil, nil
}

// FindLineageBindingsByAttemptID finds bindings by attempt ID.
func (s *SQLiteWorkflowStore) FindLineageBindingsByAttemptID(ctx context.Context, attemptID string) ([]LineageBindingRecord, error) {
	return nil, nil
}

// Close closes the database connection.
func (s *SQLiteWorkflowStore) Close() error {
	return nil
}

// NewSQLiteWorkflowStore creates a new SQLite-backed workflow store.
// This is a stub; actual implementation requires database setup.
func NewSQLiteWorkflowStore(dbPath string) (*SQLiteWorkflowStore, error) {
	return &SQLiteWorkflowStore{}, nil
}

// OpenSQLite opens a SQLite database at the given path.
// This is a stub for compatibility; actual implementation requires database setup.
func OpenSQLite(dbPath string) (*SQLiteWorkflowStore, error) {
	return NewSQLiteWorkflowStore(dbPath)
}

// Ensure SQLiteWorkflowStore implements both interfaces.
var (
	_ WorkflowStore       = (*SQLiteWorkflowStore)(nil)
	_ LineageBindingStore = (*SQLiteWorkflowStore)(nil)
)

// ErrWorkflowNotFound is returned when a workflow is not found.
var ErrWorkflowNotFound = errors.New("workflow not found")

// ErrRunNotFound is returned when a workflow run is not found.
var ErrRunNotFound = errors.New("workflow run not found")
