package rewoo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/agents/internal/workflowutil"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
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
	s.ensureCheckpointArtifactRefs(ctx, checkpointID, state)

	// Extract relevant state keys for checkpoint
	stateKeys := []string{
		"rewoo.plan_ref",
		"rewoo.tool_results_ref",
		"rewoo.tool_results_summary",
		"rewoo.replan_context",
		"rewoo.synthesis_ref",
		"rewoo.attempt",
	}
	if _, ok := state.Get("rewoo.plan_ref"); !ok {
		stateKeys = append(stateKeys, "rewoo.plan")
	}
	if _, ok := state.Get("rewoo.tool_results_ref"); !ok {
		stateKeys = append(stateKeys, "rewoo.tool_results")
	}
	if _, ok := state.Get("rewoo.synthesis_ref"); !ok {
		stateKeys = append(stateKeys, "rewoo.synthesis")
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
	if err := s.restoreArtifactBackedState(ctx, state); err != nil {
		return err
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

func (s *RewooCheckpointStore) restoreArtifactBackedState(ctx context.Context, state *core.Context) error {
	if state == nil || s == nil || s.workflowStore == nil {
		return nil
	}
	if _, ok := state.Get("rewoo.tool_results"); !ok {
		if rawRef, ok := state.Get("rewoo.tool_results_ref"); ok {
			ref, ok := rawRef.(core.ArtifactReference)
			if ok {
				var results []RewooStepResult
				if err := s.loadWorkflowArtifactJSON(ctx, ref, &results); err != nil {
					return err
				}
				state.Set("rewoo.tool_results", results)
			}
		}
	}
	if _, ok := state.Get("rewoo.plan"); !ok {
		if rawRef, ok := state.Get("rewoo.plan_ref"); ok {
			ref, ok := rawRef.(core.ArtifactReference)
			if ok {
				var plan RewooPlan
				if err := s.loadWorkflowArtifactJSON(ctx, ref, &plan); err != nil {
					return err
				}
				state.Set("rewoo.plan", &plan)
			}
		}
	}
	if _, ok := state.Get("rewoo.synthesis"); !ok {
		if rawRef, ok := state.Get("rewoo.synthesis_ref"); ok {
			ref, ok := rawRef.(core.ArtifactReference)
			if ok {
				var payload struct {
					Synthesis string `json:"synthesis"`
				}
				if err := s.loadWorkflowArtifactJSON(ctx, ref, &payload); err != nil {
					return err
				}
				if payload.Synthesis != "" {
					state.Set("rewoo.synthesis", payload.Synthesis)
				} else if ref.Summary != "" {
					state.Set("rewoo.synthesis", ref.Summary)
				}
			}
		}
	}
	return nil
}

func (s *RewooCheckpointStore) ensureCheckpointArtifactRefs(ctx context.Context, checkpointID string, state *core.Context) {
	if state == nil || s == nil || s.workflowStore == nil {
		return
	}
	workflowID := strings.TrimSpace(state.GetString("rewoo.workflow_id"))
	runID := strings.TrimSpace(state.GetString("rewoo.run_id"))
	if workflowID == "" || runID == "" {
		return
	}
	if _, ok := state.Get("rewoo.plan_ref"); !ok {
		if rawPlan, ok := state.Get("rewoo.plan"); ok && rawPlan != nil {
			if ref := s.persistPlanArtifact(ctx, checkpointID, workflowID, runID, rawPlan); ref != nil {
				state.Set("rewoo.plan_ref", *ref)
			}
		}
	}
	if _, ok := state.Get("rewoo.tool_results_ref"); !ok {
		if rawResults, ok := state.Get("rewoo.tool_results"); ok && rawResults != nil {
			if results, ok := rawResults.([]RewooStepResult); ok && len(results) > 0 {
				if ref := s.persistToolResultsArtifact(ctx, checkpointID, workflowID, runID, results); ref != nil {
					state.Set("rewoo.tool_results_ref", *ref)
					state.Set("rewoo.tool_results_summary", summarizeRewooStepResults(results))
				}
			}
		}
	}
	if _, ok := state.Get("rewoo.synthesis_ref"); !ok {
		if rawSynthesis, ok := state.Get("rewoo.synthesis"); ok && rawSynthesis != nil {
			synthesis := strings.TrimSpace(fmt.Sprint(rawSynthesis))
			if synthesis != "" && synthesis != "<nil>" {
				var results []RewooStepResult
				if rawResults, ok := state.Get("rewoo.tool_results"); ok {
					results, _ = rawResults.([]RewooStepResult)
				}
				if ref := s.persistSynthesisArtifact(ctx, checkpointID, workflowID, runID, synthesis, results); ref != nil {
					state.Set("rewoo.synthesis_ref", *ref)
				}
			}
		}
	}
}

func (s *RewooCheckpointStore) persistPlanArtifact(ctx context.Context, checkpointID, workflowID, runID string, rawPlan any) *core.ArtifactReference {
	plan, ok := rawPlan.(*RewooPlan)
	if !ok || plan == nil {
		return nil
	}
	payload, err := json.Marshal(plan)
	if err != nil {
		return nil
	}
	record := memory.WorkflowArtifactRecord{
		ArtifactID:        checkpointID + ".plan",
		WorkflowID:        workflowID,
		RunID:             runID,
		Kind:              "rewoo_plan",
		ContentType:       "application/json",
		StorageKind:       memory.ArtifactStorageInline,
		SummaryText:       strings.TrimSpace(plan.Goal),
		SummaryMetadata:   map[string]any{"checkpoint_id": checkpointID, "agent": "rewoo"},
		InlineRawText:     string(payload),
		RawSizeBytes:      int64(len(payload)),
		CompressionMethod: "none",
		CreatedAt:         time.Now().UTC(),
	}
	if err := s.workflowStore.UpsertWorkflowArtifact(ctx, record); err != nil {
		return nil
	}
	ref := workflowutil.WorkflowArtifactReference(record)
	return &ref
}

func (s *RewooCheckpointStore) persistToolResultsArtifact(ctx context.Context, checkpointID, workflowID, runID string, results []RewooStepResult) *core.ArtifactReference {
	payload, err := json.Marshal(results)
	if err != nil {
		return nil
	}
	record := memory.WorkflowArtifactRecord{
		ArtifactID:        checkpointID + ".tool_results",
		WorkflowID:        workflowID,
		RunID:             runID,
		Kind:              "rewoo_tool_results",
		ContentType:       "application/json",
		StorageKind:       memory.ArtifactStorageInline,
		SummaryText:       summarizeRewooStepResults(results),
		SummaryMetadata:   map[string]any{"checkpoint_id": checkpointID, "agent": "rewoo", "result_count": len(results)},
		InlineRawText:     string(payload),
		RawSizeBytes:      int64(len(payload)),
		CompressionMethod: "none",
		CreatedAt:         time.Now().UTC(),
	}
	if err := s.workflowStore.UpsertWorkflowArtifact(ctx, record); err != nil {
		return nil
	}
	ref := workflowutil.WorkflowArtifactReference(record)
	return &ref
}

func (s *RewooCheckpointStore) persistSynthesisArtifact(ctx context.Context, checkpointID, workflowID, runID, synthesis string, results []RewooStepResult) *core.ArtifactReference {
	payload, err := json.Marshal(map[string]any{
		"synthesis":    synthesis,
		"step_results": results,
	})
	if err != nil {
		return nil
	}
	record := memory.WorkflowArtifactRecord{
		ArtifactID:        checkpointID + ".synthesis",
		WorkflowID:        workflowID,
		RunID:             runID,
		Kind:              "rewoo_synthesis",
		ContentType:       "application/json",
		StorageKind:       memory.ArtifactStorageInline,
		SummaryText:       synthesis,
		SummaryMetadata:   map[string]any{"checkpoint_id": checkpointID, "agent": "rewoo"},
		InlineRawText:     string(payload),
		RawSizeBytes:      int64(len(payload)),
		CompressionMethod: "none",
		CreatedAt:         time.Now().UTC(),
	}
	if err := s.workflowStore.UpsertWorkflowArtifact(ctx, record); err != nil {
		return nil
	}
	ref := workflowutil.WorkflowArtifactReference(record)
	return &ref
}

func (s *RewooCheckpointStore) loadWorkflowArtifactJSON(ctx context.Context, ref core.ArtifactReference, target any) error {
	if s == nil || s.workflowStore == nil {
		return fmt.Errorf("checkpoint_restore: workflow store unavailable for artifact hydration")
	}
	artifacts, err := s.workflowStore.ListWorkflowArtifacts(ctx, ref.WorkflowID, ref.RunID)
	if err != nil {
		return fmt.Errorf("checkpoint_restore: load workflow artifacts: %w", err)
	}
	for _, artifact := range artifacts {
		if artifact.ArtifactID != ref.ArtifactID {
			continue
		}
		if artifact.InlineRawText == "" {
			return fmt.Errorf("checkpoint_restore: artifact %s has no inline payload", ref.ArtifactID)
		}
		if err := json.Unmarshal([]byte(artifact.InlineRawText), target); err != nil {
			return fmt.Errorf("checkpoint_restore: decode artifact %s: %w", ref.ArtifactID, err)
		}
		return nil
	}
	return fmt.Errorf("checkpoint_restore: artifact %s not found", ref.ArtifactID)
}
