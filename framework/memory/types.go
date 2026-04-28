// Package memory implements working memory per Section 5.5.
// Working memory is per-turn, per-session in-memory state scoped by task ID.
// Expires at checkpoint boundary. No persistence.
package memory

import (
	"context"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
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

// Evict removes a task's memory (called at checkpoint).
func (s *WorkingMemoryStore) Evict(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.tasks, taskID)
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

// TaskMemory holds entries for a single task.
type TaskMemory struct {
	taskID  string
	mu      sync.RWMutex
	entries map[string]MemoryEntry
}

// Set stores a value with the given key and memory class.
func (m *TaskMemory) Set(key string, value any, class core.MemoryClass) {
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
	}

	m.entries[key] = entry
}

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
	Class     core.MemoryClass
	CreatedAt time.Time
	UpdatedAt time.Time
}

// MemoryQuery defines query parameters for memory retrieval.
type MemoryQuery struct {
	TaskID    string
	KeyPrefix string
	Class     core.MemoryClass
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
