package memory

import (
	"context"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// Retrieve implements the MemoryRetriever interface.
// Queries memory by task ID, key prefix, memory class, with limit.
func (s *WorkingMemoryStore) Retrieve(ctx context.Context, query MemoryQuery) ([]MemoryRecordEnvelope, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[query.TaskID]
	if !ok {
		return []MemoryRecordEnvelope{}, nil
	}

	task.mu.RLock()
	defer task.mu.RUnlock()

	var results []MemoryRecordEnvelope

	for key, entry := range task.entries {
		// Check key prefix
		if query.KeyPrefix != "" && !strings.HasPrefix(key, query.KeyPrefix) {
			continue
		}

		// Check memory class
		if query.Class != "" && entry.Class != query.Class {
			continue
		}

		results = append(results, MemoryRecordEnvelope{
			TaskID: query.TaskID,
			Key:    key,
			Entry:  entry,
		})

		// Check limit
		if query.Limit > 0 && len(results) >= query.Limit {
			break
		}
	}

	return results, nil
}

// GetByKey retrieves a specific entry by task ID and key.
func (s *WorkingMemoryStore) GetByKey(taskID string, key string) (MemoryEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return MemoryEntry{}, false
	}

	return task.Get(key)
}

// GetTaskKeys returns all keys for a task.
func (s *WorkingMemoryStore) GetTaskKeys(taskID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return []string{}
	}

	return task.Keys()
}

// GetTaskSnapshot returns a snapshot of a task's memory.
func (s *WorkingMemoryStore) GetTaskSnapshot(taskID string) map[string]MemoryEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return map[string]MemoryEntry{}
	}

	return task.Snapshot()
}

// ToRetrievalReference converts a MemoryRecordEnvelope to a contextdata.RetrievalReference.
// This allows memory results to be added to an envelope's retrieval references.
func (m MemoryRecordEnvelope) ToRetrievalReference(queryID string) contextdata.RetrievalReference {
	return contextdata.RetrievalReference{
		QueryID:     queryID,
		QueryText:   "memory:" + m.TaskID + "/" + m.Key,
		Scope:       "working_memory",
		ChunkIDs:    []contextdata.ChunkID{contextdata.ChunkID(m.Key)},
		TotalFound:  1,
		FilteredOut: 0,
		RetrievedAt: time.Now().UTC(),
		Duration:    0,
	}
}

// AsRetrievalReferences converts multiple memory results to RetrievalReference slice.
// Useful for batch conversion when populating envelope retrieval references.
func AsRetrievalReferences(results []MemoryRecordEnvelope, queryID string) []contextdata.RetrievalReference {
	if len(results) == 0 {
		return nil
	}
	refs := make([]contextdata.RetrievalReference, 0, len(results))
	now := time.Now().UTC()
	for _, result := range results {
		refs = append(refs, contextdata.RetrievalReference{
			QueryID:     queryID,
			QueryText:   "memory:" + result.TaskID + "/" + result.Key,
			Scope:       "working_memory",
			ChunkIDs:    []contextdata.ChunkID{contextdata.ChunkID(result.Key)},
			TotalFound:  1,
			FilteredOut: 0,
			RetrievedAt: now,
			Duration:    0,
		})
	}
	return refs
}
