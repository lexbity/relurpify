package checkpoint

import (
	"fmt"
	"sync"

	"codeburg.org/lexbit/relurpify/framework/pipeline"
)

// Store is an in-memory checkpoint store for Phase 2.
// Production use should wrap a persistent backend (SQLite, etc.).
//
// CheckpointStore saves/loads framework/pipeline.Checkpoint objects keyed by
// (taskID, stageIndex). Supports resuming workflows after interruption.
type Store struct {
	mu          sync.RWMutex
	checkpoints map[string]*pipeline.Checkpoint // key: taskID + ":" + checkpointID
}

// NewStore creates an in-memory checkpoint store.
func NewStore() *Store {
	return &Store{
		checkpoints: make(map[string]*pipeline.Checkpoint),
	}
}

// Save persists a checkpoint.
func (s *Store) Save(checkpoint *pipeline.Checkpoint) error {
	if checkpoint == nil {
		return fmt.Errorf("checkpoint required")
	}
	if checkpoint.TaskID == "" {
		return fmt.Errorf("checkpoint task ID required")
	}
	if checkpoint.CheckpointID == "" {
		return fmt.Errorf("checkpoint ID required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.makeKey(checkpoint.TaskID, checkpoint.CheckpointID)
	s.checkpoints[key] = checkpoint
	return nil
}

// Load retrieves a checkpoint by task ID and checkpoint ID.
func (s *Store) Load(taskID, checkpointID string) (*pipeline.Checkpoint, error) {
	if taskID == "" {
		return nil, fmt.Errorf("task ID required")
	}
	if checkpointID == "" {
		return nil, fmt.Errorf("checkpoint ID required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	key := s.makeKey(taskID, checkpointID)
	cp, ok := s.checkpoints[key]
	if !ok {
		return nil, fmt.Errorf("checkpoint not found: %s", key)
	}
	return cp, nil
}

// FindLastCheckpoint returns the most recent checkpoint for a task.
// Returns nil if no checkpoints exist for the task.
func (s *Store) FindLastCheckpoint(taskID string) (*pipeline.Checkpoint, error) {
	if taskID == "" {
		return nil, fmt.Errorf("task ID required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var latest *pipeline.Checkpoint
	for key, cp := range s.checkpoints {
		// Check if key belongs to this task
		if !s.isKeyForTask(key, taskID) {
			continue
		}
		// Track latest by creation time
		if latest == nil || cp.CreatedAt.After(latest.CreatedAt) {
			latest = cp
		}
	}
	return latest, nil
}

// Clear removes all checkpoints for a task.
// Useful for cleanup after successful completion.
func (s *Store) Clear(taskID string) error {
	if taskID == "" {
		return fmt.Errorf("task ID required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	keysToDelete := []string{}
	for key := range s.checkpoints {
		if s.isKeyForTask(key, taskID) {
			keysToDelete = append(keysToDelete, key)
		}
	}
	for _, key := range keysToDelete {
		delete(s.checkpoints, key)
	}
	return nil
}

// Count returns the number of checkpoints for a task.
func (s *Store) Count(taskID string) (int, error) {
	if taskID == "" {
		return 0, fmt.Errorf("task ID required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for key := range s.checkpoints {
		if s.isKeyForTask(key, taskID) {
			count++
		}
	}
	return count, nil
}

// Helper functions

func (s *Store) makeKey(taskID, checkpointID string) string {
	return taskID + ":" + checkpointID
}

func (s *Store) isKeyForTask(key, taskID string) bool {
	if len(key) < len(taskID)+1 {
		return false
	}
	return key[:len(taskID)] == taskID && key[len(taskID)] == ':'
}
