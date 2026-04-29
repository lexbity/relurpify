// Package store provides workflow state persistence for the rex package.
package store

import (
	"context"
	"errors"
	"sync"
	"time"
)

// SQLiteWorkflowStore is a SQLite-backed implementation of WorkflowStore.
// This is a stub implementation for compilation; full persistence to be implemented.
type SQLiteWorkflowStore struct {
	mu         sync.RWMutex
	workflows  map[string]WorkflowRecord
	runs       map[string]WorkflowRunRecord
	bindings   map[string]LineageBindingRecord
	artifacts  map[string]WorkflowArtifactRecord
	events     []WorkflowEventRecord
}

// GetWorkflow retrieves a workflow by ID.
func (s *SQLiteWorkflowStore) GetWorkflow(ctx context.Context, workflowID string) (WorkflowRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.workflows[workflowID]
	return record, ok, nil
}

// CreateWorkflow creates a new workflow record.
func (s *SQLiteWorkflowStore) CreateWorkflow(ctx context.Context, record WorkflowRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensure()
	s.workflows[record.WorkflowID] = record
	return nil
}

// ListWorkflows lists workflow records, truncated by limit if positive.
func (s *SQLiteWorkflowStore) ListWorkflows(ctx context.Context, limit int) ([]WorkflowRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]WorkflowRecord, 0, len(s.workflows))
	for _, record := range s.workflows {
		out = append(out, record)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// GetRun retrieves a workflow run by ID.
func (s *SQLiteWorkflowStore) GetRun(ctx context.Context, runID string) (WorkflowRunRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.runs[runID]
	return record, ok, nil
}

// CreateRun creates a new workflow run record.
func (s *SQLiteWorkflowStore) CreateRun(ctx context.Context, record WorkflowRunRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensure()
	s.runs[record.RunID] = record
	return nil
}

// GetLineageBinding retrieves a lineage binding by workflow and run ID.
func (s *SQLiteWorkflowStore) GetLineageBinding(ctx context.Context, workflowID, runID string) (LineageBindingRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.bindings[lineageBindingKey(workflowID, runID)]
	return record, ok, nil
}

// ListWorkflowArtifacts lists artifacts for a workflow run.
func (s *SQLiteWorkflowStore) ListWorkflowArtifacts(ctx context.Context, workflowID, runID string) ([]WorkflowArtifactRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]WorkflowArtifactRecord, 0)
	for _, record := range s.artifacts {
		if record.WorkflowID != workflowID {
			continue
		}
		if runID != "" && record.RunID != runID {
			continue
		}
		out = append(out, record)
	}
	return out, nil
}

// ListEvents lists events for a workflow.
func (s *SQLiteWorkflowStore) ListEvents(ctx context.Context, workflowID string, limit int) ([]WorkflowEventRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]WorkflowEventRecord, 0)
	for _, record := range s.events {
		if record.WorkflowID != workflowID {
			continue
		}
		out = append(out, record)
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

// AppendEvent appends an event to the workflow log.
func (s *SQLiteWorkflowStore) AppendEvent(ctx context.Context, record WorkflowEventRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensure()
	s.events = append(s.events, record)
	return nil
}

// UpsertWorkflowArtifact stores or updates a workflow artifact.
func (s *SQLiteWorkflowStore) UpsertWorkflowArtifact(ctx context.Context, record WorkflowArtifactRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensure()
	s.artifacts[artifactKey(record.WorkflowID, record.RunID, record.ArtifactID)] = record
	return nil
}

// UpdateRunStatus updates the status of a workflow run.
func (s *SQLiteWorkflowStore) UpdateRunStatus(ctx context.Context, runID string, status WorkflowRunStatus, finishedAt *time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensure()
	record := s.runs[runID]
	record.Status = status
	record.FinishedAt = finishedAt
	s.runs[runID] = record
	return nil
}

// UpdateWorkflowStatus updates the status of a workflow.
func (s *SQLiteWorkflowStore) UpdateWorkflowStatus(ctx context.Context, workflowID string, version int, status WorkflowRunStatus, reason string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensure()
	record := s.workflows[workflowID]
	record.Status = status
	s.workflows[workflowID] = record
	return true, nil
}

// LineageBindingStore returns a LineageBindingStore backed by the same SQLite database.
func (s *SQLiteWorkflowStore) LineageBindingStore() LineageBindingStore {
	return s
}

// UpsertLineageBinding creates or updates a lineage binding.
func (s *SQLiteWorkflowStore) UpsertLineageBinding(ctx context.Context, record LineageBindingRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensure()
	s.bindings[lineageBindingKey(record.WorkflowID, record.RunID)] = record
	return nil
}

// FindLineageBindingsByLineageID finds bindings by lineage ID.
func (s *SQLiteWorkflowStore) FindLineageBindingsByLineageID(ctx context.Context, lineageID string) ([]LineageBindingRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LineageBindingRecord, 0)
	for _, record := range s.bindings {
		if record.LineageID == lineageID {
			out = append(out, record)
		}
	}
	return out, nil
}

// FindLineageBindingsByAttemptID finds bindings by attempt ID.
func (s *SQLiteWorkflowStore) FindLineageBindingsByAttemptID(ctx context.Context, attemptID string) ([]LineageBindingRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LineageBindingRecord, 0)
	for _, record := range s.bindings {
		if record.AttemptID == attemptID {
			out = append(out, record)
		}
	}
	return out, nil
}

// Close closes the database connection.
func (s *SQLiteWorkflowStore) Close() error {
	return nil
}

// NewSQLiteWorkflowStore creates a new SQLite-backed workflow store.
// This is a stub; actual implementation requires database setup.
func NewSQLiteWorkflowStore(dbPath string) (*SQLiteWorkflowStore, error) {
	return &SQLiteWorkflowStore{
		workflows: map[string]WorkflowRecord{},
		runs:      map[string]WorkflowRunRecord{},
		bindings:  map[string]LineageBindingRecord{},
		artifacts: map[string]WorkflowArtifactRecord{},
		events:    []WorkflowEventRecord{},
	}, nil
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

func (s *SQLiteWorkflowStore) ensure() {
	if s.workflows == nil {
		s.workflows = map[string]WorkflowRecord{}
	}
	if s.runs == nil {
		s.runs = map[string]WorkflowRunRecord{}
	}
	if s.bindings == nil {
		s.bindings = map[string]LineageBindingRecord{}
	}
	if s.artifacts == nil {
		s.artifacts = map[string]WorkflowArtifactRecord{}
	}
}

func lineageBindingKey(workflowID, runID string) string {
	return workflowID + ":" + runID
}

func artifactKey(workflowID, runID, artifactID string) string {
	return workflowID + ":" + runID + ":" + artifactID
}
