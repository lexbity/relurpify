package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/memory"
)

type SQLiteWorkflowStateStore struct {
	mu              sync.RWMutex
	path            string
	closed          bool
	workflows       map[string]*memory.WorkflowRecord
	runs            map[string]*memory.WorkflowRunRecord
	artifacts       map[string]*memory.WorkflowArtifactRecord
	events          map[string][]memory.WorkflowEventRecord
	lineageBindings map[string]*LineageBindingRecord
}

type LineageBindingRecord struct {
	WorkflowID string
	RunID      string
	LineageID  string
	AttemptID  string
	RuntimeID  string
	SessionID  string
	State      string
	UpdatedAt  time.Time
}

type LineageBindingStore interface {
	UpsertLineageBinding(context.Context, LineageBindingRecord) error
	GetLineageBinding(context.Context, string, string) (*LineageBindingRecord, bool, error)
	FindLineageBindingsByLineageID(context.Context, string) ([]LineageBindingRecord, error)
	FindLineageBindingsByAttemptID(context.Context, string) ([]LineageBindingRecord, error)
}

func NewSQLiteWorkflowStateStore(path string) (*SQLiteWorkflowStateStore, error) {
	return &SQLiteWorkflowStateStore{
		path:            filepath.Clean(path),
		workflows:       make(map[string]*memory.WorkflowRecord),
		runs:            make(map[string]*memory.WorkflowRunRecord),
		artifacts:       make(map[string]*memory.WorkflowArtifactRecord),
		events:          make(map[string][]memory.WorkflowEventRecord),
		lineageBindings: make(map[string]*LineageBindingRecord),
	}, nil
}

func (s *SQLiteWorkflowStateStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *SQLiteWorkflowStateStore) DB() *sql.DB { return nil }

func (s *SQLiteWorkflowStateStore) GetWorkflow(ctx context.Context, workflowID string) (*memory.WorkflowRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.workflows[workflowID]
	if !ok || record == nil {
		return nil, false, nil
	}
	cloned := cloneWorkflowRecord(*record)
	return &cloned, true, nil
}

func (s *SQLiteWorkflowStateStore) CreateWorkflow(ctx context.Context, record memory.WorkflowRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if record.WorkflowID == "" {
		return fmt.Errorf("workflow id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := cloneWorkflowRecord(record)
	if cloned.CreatedAt.IsZero() {
		cloned.CreatedAt = time.Now().UTC()
	}
	cloned.UpdatedAt = time.Now().UTC()
	s.workflows[record.WorkflowID] = &cloned
	return nil
}

func (s *SQLiteWorkflowStateStore) GetRun(ctx context.Context, runID string) (*memory.WorkflowRunRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.runs[runID]
	if !ok || record == nil {
		return nil, false, nil
	}
	cloned := cloneRunRecord(*record)
	return &cloned, true, nil
}

func (s *SQLiteWorkflowStateStore) CreateRun(ctx context.Context, record memory.WorkflowRunRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if record.RunID == "" {
		return fmt.Errorf("run id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := cloneRunRecord(record)
	if cloned.StartedAt.IsZero() {
		cloned.StartedAt = time.Now().UTC()
	}
	s.runs[record.RunID] = &cloned
	return nil
}

func (s *SQLiteWorkflowStateStore) ListWorkflows(ctx context.Context, limit int) ([]memory.WorkflowRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]memory.WorkflowRecord, 0, len(s.workflows))
	for _, record := range s.workflows {
		if record == nil {
			continue
		}
		out = append(out, cloneWorkflowRecord(*record))
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *SQLiteWorkflowStateStore) UpdateWorkflowMetadata(ctx context.Context, workflowID string, metadata map[string]any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.workflows[workflowID]
	if !ok || record == nil {
		return fmt.Errorf("workflow %s not found", workflowID)
	}
	if record.Metadata == nil {
		record.Metadata = map[string]any{}
	}
	for key, value := range metadata {
		record.Metadata[key] = value
	}
	record.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *SQLiteWorkflowStateStore) UpsertWorkflowArtifact(ctx context.Context, artifact memory.WorkflowArtifactRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(artifact.ArtifactID) == "" {
		return fmt.Errorf("artifact id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := cloneWorkflowArtifactRecord(artifact)
	if cloned.CreatedAt.IsZero() {
		cloned.CreatedAt = time.Now().UTC()
	}
	s.artifacts[cloned.ArtifactID] = &cloned
	return nil
}

func (s *SQLiteWorkflowStateStore) WorkflowArtifactByID(ctx context.Context, artifactID string) (*memory.WorkflowArtifactRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.artifacts[strings.TrimSpace(artifactID)]
	if !ok || record == nil {
		return nil, false, nil
	}
	cloned := cloneWorkflowArtifactRecord(*record)
	return &cloned, true, nil
}

func (s *SQLiteWorkflowStateStore) ListWorkflowArtifacts(ctx context.Context, workflowID, runID string) ([]memory.WorkflowArtifactRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	workflowID = strings.TrimSpace(workflowID)
	runID = strings.TrimSpace(runID)
	out := make([]memory.WorkflowArtifactRecord, 0, len(s.artifacts))
	for _, artifact := range s.artifacts {
		if artifact == nil {
			continue
		}
		if workflowID != "" && strings.TrimSpace(artifact.WorkflowID) != workflowID {
			continue
		}
		if runID != "" && strings.TrimSpace(artifact.RunID) != runID {
			continue
		}
		out = append(out, cloneWorkflowArtifactRecord(*artifact))
	}
	return out, nil
}

func (s *SQLiteWorkflowStateStore) ListWorkflowArtifactsByKind(ctx context.Context, workflowID, runID, kind string) ([]memory.WorkflowArtifactRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return nil, nil
	}
	artifacts, err := s.ListWorkflowArtifacts(ctx, workflowID, runID)
	if err != nil {
		return nil, err
	}
	out := make([]memory.WorkflowArtifactRecord, 0, len(artifacts))
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.Kind) == kind {
			out = append(out, artifact)
		}
	}
	return out, nil
}

func (s *SQLiteWorkflowStateStore) AppendEvent(ctx context.Context, event memory.WorkflowEventRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(event.WorkflowID) == "" {
		return fmt.Errorf("workflow id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := cloneWorkflowEventRecord(event)
	if cloned.CreatedAt.IsZero() {
		cloned.CreatedAt = time.Now().UTC()
	}
	s.events[cloned.WorkflowID] = append(s.events[cloned.WorkflowID], cloned)
	return nil
}

func (s *SQLiteWorkflowStateStore) ListEvents(ctx context.Context, workflowID string, limit int) ([]memory.WorkflowEventRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	records := append([]memory.WorkflowEventRecord(nil), s.events[strings.TrimSpace(workflowID)]...)
	if limit <= 0 || len(records) <= limit {
		return cloneWorkflowEventRecords(records), nil
	}
	return cloneWorkflowEventRecords(records[len(records)-limit:]), nil
}

func (s *SQLiteWorkflowStateStore) LatestEvent(ctx context.Context, workflowID string) (*memory.WorkflowEventRecord, bool, error) {
	records, err := s.ListEvents(ctx, workflowID, 0)
	if err != nil || len(records) == 0 {
		return nil, false, err
	}
	record := records[len(records)-1]
	return &record, true, nil
}

func (s *SQLiteWorkflowStateStore) LatestEventByTypes(ctx context.Context, workflowID string, eventTypes ...string) (*memory.WorkflowEventRecord, bool, error) {
	if len(eventTypes) == 0 {
		return nil, false, nil
	}
	records, err := s.ListEvents(ctx, workflowID, 0)
	if err != nil {
		return nil, false, err
	}
	allowed := make(map[string]struct{}, len(eventTypes))
	for _, eventType := range eventTypes {
		if trimmed := strings.TrimSpace(eventType); trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}
	for i := len(records) - 1; i >= 0; i-- {
		if _, ok := allowed[records[i].EventType]; ok {
			record := records[i]
			return &record, true, nil
		}
	}
	return nil, false, nil
}

func (s *SQLiteWorkflowStateStore) UpsertLineageBinding(ctx context.Context, record LineageBindingRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key := lineageBindingKey(record.WorkflowID, record.RunID)
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := cloneLineageBindingRecord(record)
	if cloned.UpdatedAt.IsZero() {
		cloned.UpdatedAt = time.Now().UTC()
	}
	s.lineageBindings[key] = &cloned
	return nil
}

func (s *SQLiteWorkflowStateStore) GetLineageBinding(ctx context.Context, workflowID, runID string) (*LineageBindingRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.lineageBindings[lineageBindingKey(workflowID, runID)]
	if !ok || record == nil {
		return nil, false, nil
	}
	cloned := cloneLineageBindingRecord(*record)
	return &cloned, true, nil
}

func (s *SQLiteWorkflowStateStore) FindLineageBindingsByLineageID(ctx context.Context, lineageID string) ([]LineageBindingRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LineageBindingRecord, 0, len(s.lineageBindings))
	for _, record := range s.lineageBindings {
		if record == nil || strings.TrimSpace(record.LineageID) != strings.TrimSpace(lineageID) {
			continue
		}
		out = append(out, cloneLineageBindingRecord(*record))
	}
	return out, nil
}

func (s *SQLiteWorkflowStateStore) FindLineageBindingsByAttemptID(ctx context.Context, attemptID string) ([]LineageBindingRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LineageBindingRecord, 0, len(s.lineageBindings))
	for _, record := range s.lineageBindings {
		if record == nil || strings.TrimSpace(record.AttemptID) != strings.TrimSpace(attemptID) {
			continue
		}
		out = append(out, cloneLineageBindingRecord(*record))
	}
	return out, nil
}

func cloneWorkflowRecord(record memory.WorkflowRecord) memory.WorkflowRecord {
	cloned := record
	if record.Metadata != nil {
		cloned.Metadata = make(map[string]any, len(record.Metadata))
		for key, value := range record.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return cloned
}

func cloneRunRecord(record memory.WorkflowRunRecord) memory.WorkflowRunRecord {
	cloned := record
	if record.Metadata != nil {
		cloned.Metadata = make(map[string]any, len(record.Metadata))
		for key, value := range record.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return cloned
}

func cloneWorkflowArtifactRecord(record memory.WorkflowArtifactRecord) memory.WorkflowArtifactRecord {
	cloned := record
	if record.SummaryMetadata != nil {
		cloned.SummaryMetadata = make(map[string]any, len(record.SummaryMetadata))
		for key, value := range record.SummaryMetadata {
			cloned.SummaryMetadata[key] = value
		}
	}
	return cloned
}

func cloneWorkflowEventRecord(record memory.WorkflowEventRecord) memory.WorkflowEventRecord {
	cloned := record
	if record.Metadata != nil {
		cloned.Metadata = make(map[string]any, len(record.Metadata))
		for key, value := range record.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return cloned
}

func cloneWorkflowEventRecords(records []memory.WorkflowEventRecord) []memory.WorkflowEventRecord {
	if len(records) == 0 {
		return nil
	}
	out := make([]memory.WorkflowEventRecord, 0, len(records))
	for _, record := range records {
		out = append(out, cloneWorkflowEventRecord(record))
	}
	return out
}

func lineageBindingKey(workflowID, runID string) string {
	return strings.TrimSpace(workflowID) + "\x00" + strings.TrimSpace(runID)
}

func cloneLineageBindingRecord(record LineageBindingRecord) LineageBindingRecord {
	return record
}
