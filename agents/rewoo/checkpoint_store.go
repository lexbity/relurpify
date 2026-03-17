package rewoo

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
)

// CheckpointKey represents a unique identifier for a checkpoint.
type CheckpointKey string

// CheckpointMetadata contains information about a checkpoint.
type CheckpointMetadata struct {
	CheckpointID  string                 `json:"checkpoint_id"`
	Phase         string                 `json:"phase"`
	Attempt       int                    `json:"attempt"`
	Timestamp     time.Time              `json:"timestamp"`
	StepsExecuted []string               `json:"steps_executed"`
	StateKeys     []string               `json:"state_keys"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// RewooCheckpointStore persists execution state for recovery.
// It implements checkpoint persistence for workflow pause/resume and error recovery.
// Phase 7: Currently stores checkpoints in-memory; future phases can wire to persistent storage.
type RewooCheckpointStore struct {
	workflowStore memory.WorkflowStateStore
	checkpoints   map[string]*CheckpointMetadata // In-memory checkpoint storage
	mu            sync.RWMutex
	debugf        func(string, ...interface{})
}

// NewRewooCheckpointStore creates a new checkpoint store.
// Phase 7: Currently in-memory; can be extended to persistent storage in future phases.
func NewRewooCheckpointStore(workflowStore memory.WorkflowStateStore, debugf func(string, ...interface{})) *RewooCheckpointStore {
	if debugf == nil {
		debugf = func(string, ...interface{}) {}
	}
	return &RewooCheckpointStore{
		workflowStore: workflowStore,
		checkpoints:   make(map[string]*CheckpointMetadata),
		debugf:        debugf,
	}
}

// SaveCheckpoint persists execution state at a checkpoint.
// The checkpoint key is typically "rewoo.<phase>.<attempt>".
// Phase 7: Currently stores in-memory; can be persisted to workflow store in future phases.
func (s *RewooCheckpointStore) SaveCheckpoint(ctx context.Context, checkpointID string, phase string, attempt int, state *core.Context) error {
	// Extract relevant state keys for checkpoint
	stateKeys := []string{
		"rewoo.plan",
		"rewoo.tool_results",
		"rewoo.replan_context",
		"rewoo.synthesis",
		"rewoo.attempt",
	}

	stateSnapshot := make(map[string]interface{})
	for _, key := range stateKeys {
		if val, ok := state.Get(key); ok {
			stateSnapshot[key] = val
		}
	}

	// Extract executed steps
	stepsExecuted := extractExecutedSteps(state)

	// Create checkpoint metadata
	metadata := &CheckpointMetadata{
		CheckpointID:  checkpointID,
		Phase:         phase,
		Attempt:       attempt,
		Timestamp:     time.Now().UTC(),
		StepsExecuted: stepsExecuted,
		StateKeys:     stateKeys,
		Metadata: map[string]interface{}{
			"state_snapshot": stateSnapshot,
		},
	}

	// Store in-memory
	s.mu.Lock()
	s.checkpoints[checkpointID] = metadata
	s.mu.Unlock()

	// Phase 7 Note: Persistence to workflow store can be added in future phases
	// For now, checkpoints live in-memory during execution

	s.debugf("saved checkpoint %s at phase %s attempt %d", checkpointID, phase, attempt)
	return nil
}

// LoadCheckpoint restores execution state from a checkpoint.
// Phase 7: Currently loads from in-memory storage.
func (s *RewooCheckpointStore) LoadCheckpoint(ctx context.Context, checkpointID string) (*CheckpointMetadata, error) {
	s.mu.RLock()
	metadata, ok := s.checkpoints[checkpointID]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("checkpoint_load: checkpoint %s not found", checkpointID)
	}

	s.debugf("loaded checkpoint %s from phase %s attempt %d", checkpointID, metadata.Phase, metadata.Attempt)
	return metadata, nil
}

// RestoreStateFromCheckpoint applies a checkpoint's state to the execution context.
func (s *RewooCheckpointStore) RestoreStateFromCheckpoint(ctx context.Context, state *core.Context, checkpoint *CheckpointMetadata) error {
	if checkpoint == nil || checkpoint.Metadata == nil {
		return fmt.Errorf("checkpoint_restore: invalid checkpoint")
	}

	// Extract snapshot from metadata
	stateSnapshot, ok := checkpoint.Metadata["state_snapshot"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("checkpoint_restore: no state snapshot in checkpoint")
	}

	// Apply snapshot to execution state
	for key, val := range stateSnapshot {
		state.Set(key, val)
	}

	state.Set("rewoo.checkpoint_loaded", checkpoint.CheckpointID)
	state.Set("rewoo.checkpoint_phase", checkpoint.Phase)
	state.Set("rewoo.checkpoint_attempt", checkpoint.Attempt)

	s.debugf("restored state from checkpoint %s", checkpoint.CheckpointID)
	return nil
}

// ListCheckpoints returns all available checkpoints.
func (s *RewooCheckpointStore) ListCheckpoints(ctx context.Context) ([]CheckpointMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var checkpoints []CheckpointMetadata
	for _, metadata := range s.checkpoints {
		checkpoints = append(checkpoints, *metadata)
	}

	return checkpoints, nil
}

// DeleteCheckpoint removes a checkpoint.
func (s *RewooCheckpointStore) DeleteCheckpoint(ctx context.Context, checkpointID string) error {
	s.mu.Lock()
	delete(s.checkpoints, checkpointID)
	s.mu.Unlock()

	s.debugf("deleted checkpoint %s", checkpointID)
	return nil
}

// extractExecutedSteps returns the list of steps that have been executed.
func extractExecutedSteps(state *core.Context) []string {
	var executed []string

	// Extract step IDs from aggregated results
	if resultsVal, ok := state.Get("rewoo.tool_results"); ok {
		if results, ok := resultsVal.([]RewooStepResult); ok {
			for _, result := range results {
				executed = append(executed, result.StepID)
			}
		}
	}

	return executed
}
