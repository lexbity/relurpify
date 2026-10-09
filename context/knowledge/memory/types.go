// Package memory implements working memory per Section 5.5.
// Working memory is per-turn, per-session in-memory state scoped by task ID.
// Expires at checkpoint boundary. No persistence.
package memory

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	relurpctx "codeburg.org/lexbit/relurpify/context"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/governance/identity"
)

// WorkingMemoryStore holds per-task ephemeral state.
type WorkingMemoryStore struct {
	mu    sync.RWMutex
	tasks map[string]*TaskMemory
}

// NewWorkingMemoryStore creates a new working memory store.
func NewWorkingMemoryStore() *WorkingMemoryStore {
	return &WorkingMemoryStore{
		tasks: make(map[string]*TaskMemory),
	}
}

// Scope returns or creates a task-scoped memory.
func (s *WorkingMemoryStore) Scope(taskID string) *TaskMemory {
	s.mu.Lock()
	defer s.mu.Unlock()

	if task, ok := s.tasks[taskID]; ok {
		return task
	}

	task := &TaskMemory{
		taskID:  taskID,
		entries: make(map[string]MemoryEntry),
	}
	s.tasks[taskID] = task
	return task
}

// Evict removes a task's memory entirely: the task leaves the store and its
// entries are dropped, so held pointers read empty and the memory is
// released. Called at checkpoint boundaries and task completion. Idempotent.
func (s *WorkingMemoryStore) Evict(taskID string) {
	s.mu.Lock()
	task, ok := s.tasks[taskID]
	if ok {
		delete(s.tasks, taskID)
	}
	s.mu.Unlock()
	if ok {
		task.clear()
	}
}

// clear drops every entry from this task memory.
func (m *TaskMemory) clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = make(map[string]MemoryEntry)
	m.order = nil
}

// ListTasks returns all active task IDs.
func (s *WorkingMemoryStore) ListTasks() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]string, 0, len(s.tasks))
	for taskID := range s.tasks {
		result = append(result, taskID)
	}
	return result
}

// taskMemoryMaxEntries bounds one task's working memory. Past the cap the
// oldest entry is evicted; the eviction is counted on the store.
const taskMemoryMaxEntries = 1024

// TaskMemory holds entries for a single task.
type TaskMemory struct {
	taskID  string
	mu      sync.RWMutex
	entries map[string]MemoryEntry
	// order tracks insertion order for oldest-entry eviction at the cap.
	order []string
}

// Set stores a value with the given key and memory class.
func (m *TaskMemory) Set(key string, value any, class relurpctx.MemoryClass) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	existing, exists := m.entries[key]

	entry := MemoryEntry{
		Value:     value,
		Class:     class,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if exists {
		entry.CreatedAt = existing.CreatedAt
	} else {
		m.order = append(m.order, key)
	}

	m.entries[key] = entry
	for len(m.entries) > taskMemoryMaxEntries {
		oldest := m.order[0]
		m.order = m.order[1:]
		delete(m.entries, oldest)
		workingMemoryEvictions.Add(1)
	}
}

// workingMemoryEvictions counts per-task cap evictions across the process
// (the eviction counter contract: no bound is silent).
var workingMemoryEvictions atomic.Int64

// Evictions reports the process-wide count of per-task cap evictions.
func Evictions() int64 { return workingMemoryEvictions.Load() }

// Get retrieves a value by key.
func (m *TaskMemory) Get(key string) (MemoryEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.entries[key]
	return entry, ok
}

// Keys returns all keys in this task memory.
func (m *TaskMemory) Keys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]string, 0, len(m.entries))
	for key := range m.entries {
		result = append(result, key)
	}
	return result
}

// Snapshot returns a point-in-time copy of all entries.
func (m *TaskMemory) Snapshot() map[string]MemoryEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]MemoryEntry, len(m.entries))
	for k, v := range m.entries {
		result[k] = v
	}
	return result
}

// Delete removes an entry.
func (m *TaskMemory) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.entries, key)
}

// Clear removes all entries.
func (m *TaskMemory) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.entries = make(map[string]MemoryEntry)
}

// TaskID returns the task ID.
func (m *TaskMemory) TaskID() string {
	return m.taskID
}

// MemoryEntry stores a single memory value.
type MemoryEntry struct {
	Value     any
	Class     relurpctx.MemoryClass
	CreatedAt time.Time
	UpdatedAt time.Time
}

// MemoryQuery defines query parameters for memory retrieval.
type MemoryQuery struct {
	TaskID    string
	KeyPrefix string
	Class     relurpctx.MemoryClass
	Limit     int
}

// MemoryRecordEnvelope is the result of a memory query.
type MemoryRecordEnvelope struct {
	TaskID string
	Key    string
	Entry  MemoryEntry
}

// MemoryRetriever is the interface agentgraph nodes use to query memory.
type MemoryRetriever interface {
	Retrieve(ctx context.Context, query MemoryQuery) ([]MemoryRecordEnvelope, error)
}

// StateHydrator populates graph execution state from memory retrieval results.
type StateHydrator interface {
	Hydrate(ctx context.Context, state map[string]any, results []MemoryRecordEnvelope) error
}

// EnvelopeHydrator populates contextdata.Envelope from memory retrieval results.
// This is the preferred interface for the tiered context model.
type EnvelopeHydrator interface {
	HydrateIntoEnvelope(ctx context.Context, env *contextdata.Envelope, results []MemoryRecordEnvelope) error
}

// PromotionRequest carries a working memory entry to be durably persisted.
type PromotionRequest struct {
	TaskID      string
	Key         string
	Destination knowledge.SourceOrigin
	Principal   identity.SubjectRef
	Reason      string
}
