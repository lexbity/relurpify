package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type HybridMemory struct {
	*CompositeRuntimeStore
}

type HybridWorkflowStateStore struct {
	mu        sync.RWMutex
	workflows map[string]*WorkflowRecord
	runs      map[string]*WorkflowRunRecord
	artifacts map[string]*WorkflowArtifactRecord
	events    map[string][]WorkflowEventRecord
}

func NewHybridMemory(_ string) (*HybridMemory, error) {
	return &HybridMemory{
		CompositeRuntimeStore: &CompositeRuntimeStore{
			WorkflowStateStore: NewHybridWorkflowStateStore(),
			memoryRecords:      make(map[string]map[MemoryScope]MemoryRecord),
		},
	}, nil
}

func NewHybridWorkflowStateStore() *HybridWorkflowStateStore {
	return &HybridWorkflowStateStore{
		workflows: make(map[string]*WorkflowRecord),
		runs:      make(map[string]*WorkflowRunRecord),
		artifacts: make(map[string]*WorkflowArtifactRecord),
		events:    make(map[string][]WorkflowEventRecord),
	}
}

func (s *HybridWorkflowStateStore) Close() error { return nil }

func (s *HybridWorkflowStateStore) GetWorkflow(_ context.Context, workflowID string) (*WorkflowRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.workflows[strings.TrimSpace(workflowID)]
	if !ok || record == nil {
		return nil, false, nil
	}
	cloned := cloneWorkflowRecord(*record)
	return &cloned, true, nil
}

func (s *HybridWorkflowStateStore) CreateWorkflow(_ context.Context, record WorkflowRecord) error {
	if strings.TrimSpace(record.WorkflowID) == "" {
		return fmt.Errorf("workflow id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := cloneWorkflowRecord(record)
	if cloned.CreatedAt.IsZero() {
		cloned.CreatedAt = time.Now().UTC()
	}
	cloned.UpdatedAt = time.Now().UTC()
	s.workflows[cloned.WorkflowID] = &cloned
	return nil
}

func (s *HybridWorkflowStateStore) GetRun(_ context.Context, runID string) (*WorkflowRunRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.runs[strings.TrimSpace(runID)]
	if !ok || record == nil {
		return nil, false, nil
	}
	cloned := cloneRunRecord(*record)
	return &cloned, true, nil
}

func (s *HybridWorkflowStateStore) CreateRun(_ context.Context, record WorkflowRunRecord) error {
	if strings.TrimSpace(record.RunID) == "" {
		return fmt.Errorf("run id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := cloneRunRecord(record)
	if cloned.StartedAt.IsZero() {
		cloned.StartedAt = time.Now().UTC()
	}
	s.runs[cloned.RunID] = &cloned
	return nil
}

func (s *HybridWorkflowStateStore) UpdateWorkflowMetadata(_ context.Context, workflowID string, metadata map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.workflows[strings.TrimSpace(workflowID)]
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

func (s *HybridWorkflowStateStore) ListWorkflows(_ context.Context, limit int) ([]WorkflowRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]WorkflowRecord, 0, len(s.workflows))
	for _, record := range s.workflows {
		if record == nil {
			continue
		}
		out = append(out, cloneWorkflowRecord(*record))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].WorkflowID < out[j].WorkflowID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *HybridWorkflowStateStore) UpsertWorkflowArtifact(_ context.Context, artifact WorkflowArtifactRecord) error {
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

func (s *HybridWorkflowStateStore) WorkflowArtifactByID(_ context.Context, artifactID string) (*WorkflowArtifactRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.artifacts[strings.TrimSpace(artifactID)]
	if !ok || record == nil {
		return nil, false, nil
	}
	cloned := cloneWorkflowArtifactRecord(*record)
	return &cloned, true, nil
}

func (s *HybridWorkflowStateStore) ListWorkflowArtifacts(_ context.Context, workflowID, runID string) ([]WorkflowArtifactRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	workflowID = strings.TrimSpace(workflowID)
	runID = strings.TrimSpace(runID)
	out := make([]WorkflowArtifactRecord, 0, len(s.artifacts))
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

func (s *HybridWorkflowStateStore) ListWorkflowArtifactsByKind(ctx context.Context, workflowID, runID, kind string) ([]WorkflowArtifactRecord, error) {
	if strings.TrimSpace(kind) == "" {
		return nil, nil
	}
	artifacts, err := s.ListWorkflowArtifacts(ctx, workflowID, runID)
	if err != nil {
		return nil, err
	}
	out := make([]WorkflowArtifactRecord, 0, len(artifacts))
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.Kind) == strings.TrimSpace(kind) {
			out = append(out, artifact)
		}
	}
	return out, nil
}

func (s *HybridWorkflowStateStore) AppendEvent(_ context.Context, event WorkflowEventRecord) error {
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

func (s *HybridWorkflowStateStore) ListEvents(_ context.Context, workflowID string, limit int) ([]WorkflowEventRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	records := append([]WorkflowEventRecord(nil), s.events[strings.TrimSpace(workflowID)]...)
	if limit > 0 && len(records) > limit {
		records = records[len(records)-limit:]
	}
	return cloneWorkflowEventRecords(records), nil
}

func (s *HybridWorkflowStateStore) LatestEvent(ctx context.Context, workflowID string) (*WorkflowEventRecord, bool, error) {
	records, err := s.ListEvents(ctx, workflowID, 0)
	if err != nil || len(records) == 0 {
		return nil, false, err
	}
	record := records[len(records)-1]
	return &record, true, nil
}

func (s *HybridWorkflowStateStore) LatestEventByTypes(ctx context.Context, workflowID string, eventTypes ...string) (*WorkflowEventRecord, bool, error) {
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

func cloneWorkflowRecord(record WorkflowRecord) WorkflowRecord {
	cloned := record
	if record.Metadata != nil {
		cloned.Metadata = make(map[string]any, len(record.Metadata))
		for key, value := range record.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return cloned
}

func cloneRunRecord(record WorkflowRunRecord) WorkflowRunRecord {
	cloned := record
	if record.Metadata != nil {
		cloned.Metadata = make(map[string]any, len(record.Metadata))
		for key, value := range record.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return cloned
}

func cloneWorkflowArtifactRecord(record WorkflowArtifactRecord) WorkflowArtifactRecord {
	cloned := record
	if record.SummaryMetadata != nil {
		cloned.SummaryMetadata = make(map[string]any, len(record.SummaryMetadata))
		for key, value := range record.SummaryMetadata {
			cloned.SummaryMetadata[key] = value
		}
	}
	return cloned
}

func cloneWorkflowEventRecord(record WorkflowEventRecord) WorkflowEventRecord {
	cloned := record
	if record.Metadata != nil {
		cloned.Metadata = make(map[string]any, len(record.Metadata))
		for key, value := range record.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return cloned
}

func cloneWorkflowEventRecords(records []WorkflowEventRecord) []WorkflowEventRecord {
	if len(records) == 0 {
		return nil
	}
	out := make([]WorkflowEventRecord, 0, len(records))
	for _, record := range records {
		out = append(out, cloneWorkflowEventRecord(record))
	}
	return out
}

func (s *CompositeRuntimeStore) WithVectorStore(store VectorStore) MemoryStore {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vectorStore = store
	return s
}

func (s *CompositeRuntimeStore) Remember(_ context.Context, key string, value map[string]any, scope MemoryScope) error {
	if s == nil {
		return nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("memory key required")
	}
	now := time.Now().UTC()
	record := MemoryRecord{Key: key, Value: cloneAnyMap(value), Scope: scope, UpdatedAt: now}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.memoryRecords == nil {
		s.memoryRecords = make(map[string]map[MemoryScope]MemoryRecord)
	}
	if existing, ok := s.memoryRecords[key]; ok {
		if prior, ok := existing[scope]; ok {
			record.CreatedAt = prior.CreatedAt
		}
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if _, ok := s.memoryRecords[key]; !ok {
		s.memoryRecords[key] = make(map[MemoryScope]MemoryRecord)
	}
	s.memoryRecords[key][scope] = record
	return nil
}

func (s *CompositeRuntimeStore) Recall(_ context.Context, key string, scope MemoryScope) (*MemoryRecord, bool, error) {
	if s == nil {
		return nil, false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if records, ok := s.memoryRecords[strings.TrimSpace(key)]; ok {
		if record, ok := records[scope]; ok {
			cloned := record
			cloned.Value = cloneAnyMap(record.Value)
			return &cloned, true, nil
		}
	}
	return nil, false, nil
}

func (s *CompositeRuntimeStore) Search(_ context.Context, query string, scope MemoryScope) ([]MemoryRecord, error) {
	if s == nil {
		return nil, nil
	}
	query = strings.ToLower(strings.TrimSpace(query))
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MemoryRecord, 0)
	for _, byScope := range s.memoryRecords {
		for recordScope, record := range byScope {
			if scope != "" && recordScope != scope {
				continue
			}
			if query != "" && !strings.Contains(strings.ToLower(record.Key), query) {
				continue
			}
			cloned := record
			cloned.Value = cloneAnyMap(record.Value)
			out = append(out, cloned)
		}
	}
	return out, nil
}

func (s *CompositeRuntimeStore) Forget(_ context.Context, key string, scope MemoryScope) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if records, ok := s.memoryRecords[strings.TrimSpace(key)]; ok {
		delete(records, scope)
		if len(records) == 0 {
			delete(s.memoryRecords, strings.TrimSpace(key))
		}
	}
	return nil
}

func (s *CompositeRuntimeStore) Summarize(_ context.Context, scope MemoryScope) (string, error) {
	if s == nil {
		return "", nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, byScope := range s.memoryRecords {
		for recordScope := range byScope {
			if scope == "" || recordScope == scope {
				count++
			}
		}
	}
	return fmt.Sprintf("%d memory records", count), nil
}

func cloneAnyMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}
