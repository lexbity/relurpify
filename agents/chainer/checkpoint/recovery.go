package checkpoint

import (
	"fmt"

	"github.com/lexcodex/relurpify/framework/pipeline"
)

// RecoveryManager handles resuming interrupted workflows.
//
// It queries a CheckpointStore to find the last completed stage for a task,
// then provides a ResumeCheckpoint that the pipeline runner can use to skip
// already-completed stages and continue from where it left off.
type RecoveryManager struct {
	Store pipeline.CheckpointStore
}

// NewRecoveryManager creates a recovery manager wrapping a checkpoint store.
func NewRecoveryManager(store pipeline.CheckpointStore) *RecoveryManager {
	return &RecoveryManager{Store: store}
}

// FindLastCheckpoint queries the store for the most recent checkpoint for a task.
//
// Returns:
//   - A ResumeCheckpoint ready to pass to pipeline.Runner.Options.ResumeCheckpoint
//   - nil if no checkpoints exist for the task (resume not needed)
//   - error if query fails
//
// The caller should pass the returned checkpoint to the Runner if non-nil.
func (m *RecoveryManager) FindLastCheckpoint(taskID string) (*pipeline.Checkpoint, error) {
	if m.Store == nil {
		return nil, fmt.Errorf("recovery: checkpoint store not configured")
	}
	if taskID == "" {
		return nil, fmt.Errorf("recovery: task ID required")
	}

	// Query for the most recent checkpoint
	// Phase 2: Using in-memory store with FindLastCheckpoint method
	// Phase 3+: May add persistent backend query logic
	if store, ok := m.Store.(*Store); ok {
		return store.FindLastCheckpoint(taskID)
	}

	// Fallback: if using different store type, no resume available
	return nil, nil
}

// HasCheckpoints returns true if any checkpoints exist for a task.
func (m *RecoveryManager) HasCheckpoints(taskID string) (bool, error) {
	if m.Store == nil {
		return false, fmt.Errorf("recovery: checkpoint store not configured")
	}
	if taskID == "" {
		return false, fmt.Errorf("recovery: task ID required")
	}

	if store, ok := m.Store.(*Store); ok {
		count, err := store.Count(taskID)
		return count > 0, err
	}

	return false, nil
}

// ClearCheckpoints removes all checkpoints for a task after successful completion.
// Useful to prevent stale resumption data from blocking fresh executions.
func (m *RecoveryManager) ClearCheckpoints(taskID string) error {
	if m.Store == nil {
		return fmt.Errorf("recovery: checkpoint store not configured")
	}
	if taskID == "" {
		return fmt.Errorf("recovery: task ID required")
	}

	if store, ok := m.Store.(*Store); ok {
		return store.Clear(taskID)
	}

	// Fallback: non-clearable store type
	return nil
}

// MustFindCheckpoint wraps FindLastCheckpoint and panics on error.
// Use only in tests or when error is truly unrecoverable.
func (m *RecoveryManager) MustFindCheckpoint(taskID string) *pipeline.Checkpoint {
	cp, err := m.FindLastCheckpoint(taskID)
	if err != nil {
		panic(err)
	}
	return cp
}
