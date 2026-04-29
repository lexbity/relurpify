package rewoo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	frameworkpersistence "codeburg.org/lexbit/relurpify/framework/persistence"
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
	lifecycleRepo agentlifecycle.Repository
	checkpoints   map[string]*CheckpointMetadata // In-memory checkpoint storage
	mu            sync.RWMutex
	debugf        func(string, ...interface{})
}

// NewRewooCheckpointStore creates a new checkpoint store.
// Phase 7: Currently in-memory; can be extended to persistent storage in future phases.
func NewRewooCheckpointStore(lifecycleRepo agentlifecycle.Repository, debugf func(string, ...interface{})) *RewooCheckpointStore {
	if debugf == nil {
		debugf = func(string, ...interface{}) {}
	}
	return &RewooCheckpointStore{
		lifecycleRepo: lifecycleRepo,
		checkpoints:   make(map[string]*CheckpointMetadata),
		debugf:        debugf,
	}
}

// SaveCheckpoint persists execution state at a checkpoint.
// The checkpoint key is typically "rewoo.<phase>.<attempt>".
// Phase 7: Currently stores in-memory; can be persisted to workflow store in future phases.
func (s *RewooCheckpointStore) SaveCheckpoint(ctx context.Context, checkpointID string, phase string, attempt int, env *contextdata.Envelope) error {
	s.ensureCheckpointArtifactRefs(ctx, checkpointID, env)
	workflowID := envGetString(env, "rewoo.workflow_id")
	runID := envGetString(env, "rewoo.run_id")

	// Extract relevant state keys for checkpoint
	stateKeys := []string{
		"rewoo.plan_ref",
		"rewoo.tool_results_ref",
		"rewoo.tool_results_summary",
		"rewoo.replan_context",
		"rewoo.synthesis_ref",
		"rewoo.attempt",
	}
	if _, ok := env.GetWorkingValue("rewoo.plan_ref"); !ok {
		stateKeys = append(stateKeys, "rewoo.plan")
	}
	if _, ok := env.GetWorkingValue("rewoo.tool_results_ref"); !ok {
		stateKeys = append(stateKeys, "rewoo.tool_results")
	}
	if _, ok := env.GetWorkingValue("rewoo.synthesis_ref"); !ok {
		stateKeys = append(stateKeys, "rewoo.synthesis")
	}

	stateSnapshot := make(map[string]interface{})
	for _, key := range stateKeys {
		if val, ok := env.GetWorkingValue(key); ok {
			stateSnapshot[key] = val
		}
	}

	// Extract executed steps
	stepsExecuted := extractExecutedSteps(env)

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

	if ref, err := frameworkpersistence.SaveCheckpointArtifact(ctx, env, s.lifecycleRepo, frameworkpersistence.CheckpointSnapshot{
		CheckpointID: checkpointID,
		WorkflowID:   workflowID,
		RunID:        runID,
		Kind:         "rewoo_checkpoint",
		Summary:      fmt.Sprintf("%s phase %s attempt %d", checkpointID, phase, attempt),
		Metadata: map[string]any{
			"phase":          phase,
			"attempt":        attempt,
			"state_snapshot": stateSnapshot,
		},
		InlineRaw: string(mustJSON(metadata)),
	}); err == nil && ref != nil {
		env.SetWorkingValue("rewoo.checkpoint_ref", *ref, contextdata.MemoryClassTask)
	}

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
func (s *RewooCheckpointStore) RestoreStateFromCheckpoint(ctx context.Context, env *contextdata.Envelope, checkpoint *CheckpointMetadata) error {
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
		env.SetWorkingValue(key, val, contextdata.MemoryClassTask)
	}
	if err := s.restoreArtifactBackedState(ctx, env); err != nil {
		return err
	}

	env.SetWorkingValue("rewoo.checkpoint_loaded", checkpoint.CheckpointID, contextdata.MemoryClassTask)
	env.SetWorkingValue("rewoo.checkpoint_phase", checkpoint.Phase, contextdata.MemoryClassTask)
	env.SetWorkingValue("rewoo.checkpoint_attempt", checkpoint.Attempt, contextdata.MemoryClassTask)

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
func extractExecutedSteps(env *contextdata.Envelope) []string {
	var executed []string

	// Extract step IDs from aggregated results
	if resultsVal, ok := env.GetWorkingValue("rewoo.tool_results"); ok {
		if results, ok := resultsVal.([]RewooStepResult); ok {
			for _, result := range results {
				executed = append(executed, result.StepID)
			}
		}
	}

	return executed
}

func (s *RewooCheckpointStore) restoreArtifactBackedState(ctx context.Context, env *contextdata.Envelope) error {
	if env == nil || s == nil || s.lifecycleRepo == nil {
		return nil
	}
	if _, ok := env.GetWorkingValue("rewoo.tool_results"); !ok {
		if rawRef, ok := env.GetWorkingValue("rewoo.tool_results_ref"); ok {
			ref, ok := rawRef.(core.ArtifactReference)
			if ok {
				var results []RewooStepResult
				if err := s.loadWorkflowArtifactJSON(ctx, ref, &results); err != nil {
					return err
				}
				env.SetWorkingValue("rewoo.tool_results", results, contextdata.MemoryClassTask)
			}
		}
	}
	if _, ok := env.GetWorkingValue("rewoo.plan"); !ok {
		if rawRef, ok := env.GetWorkingValue("rewoo.plan_ref"); ok {
			ref, ok := rawRef.(core.ArtifactReference)
			if ok {
				var plan RewooPlan
				if err := s.loadWorkflowArtifactJSON(ctx, ref, &plan); err != nil {
					return err
				}
				env.SetWorkingValue("rewoo.plan", &plan, contextdata.MemoryClassTask)
			}
		}
	}
	if _, ok := env.GetWorkingValue("rewoo.synthesis"); !ok {
		if rawRef, ok := env.GetWorkingValue("rewoo.synthesis_ref"); ok {
			ref, ok := rawRef.(core.ArtifactReference)
			if ok {
				var payload struct {
					Synthesis string `json:"synthesis"`
				}
				if err := s.loadWorkflowArtifactJSON(ctx, ref, &payload); err != nil {
					return err
				}
				if payload.Synthesis != "" {
					env.SetWorkingValue("rewoo.synthesis", payload.Synthesis, contextdata.MemoryClassTask)
				} else if ref.Summary != "" {
					env.SetWorkingValue("rewoo.synthesis", ref.Summary, contextdata.MemoryClassTask)
				}
			}
		}
	}
	return nil
}

func (s *RewooCheckpointStore) ensureCheckpointArtifactRefs(ctx context.Context, checkpointID string, env *contextdata.Envelope) {
	if env == nil || s == nil || s.lifecycleRepo == nil {
		return
	}
	workflowID := envGetString(env, "rewoo.workflow_id")
	runID := envGetString(env, "rewoo.run_id")
	if workflowID == "" || runID == "" {
		return
	}
	if _, ok := env.GetWorkingValue("rewoo.plan_ref"); !ok {
		if rawPlan, ok := env.GetWorkingValue("rewoo.plan"); ok && rawPlan != nil {
			if ref := s.persistPlanArtifact(ctx, checkpointID, workflowID, runID, rawPlan); ref != nil {
				env.SetWorkingValue("rewoo.plan_ref", *ref, contextdata.MemoryClassTask)
			}
		}
	}
	if _, ok := env.GetWorkingValue("rewoo.tool_results_ref"); !ok {
		if rawResults, ok := env.GetWorkingValue("rewoo.tool_results"); ok && rawResults != nil {
			if results, ok := rawResults.([]RewooStepResult); ok && len(results) > 0 {
				if ref := s.persistToolResultsArtifact(ctx, checkpointID, workflowID, runID, results); ref != nil {
					env.SetWorkingValue("rewoo.tool_results_ref", *ref, contextdata.MemoryClassTask)
					env.SetWorkingValue("rewoo.tool_results_summary", summarizeRewooStepResults(results), contextdata.MemoryClassTask)
				}
			}
		}
	}
	if _, ok := env.GetWorkingValue("rewoo.synthesis_ref"); !ok {
		if rawSynthesis, ok := env.GetWorkingValue("rewoo.synthesis"); ok && rawSynthesis != nil {
			synthesis := strings.TrimSpace(fmt.Sprint(rawSynthesis))
			if synthesis != "" && synthesis != "<nil>" {
				var results []RewooStepResult
				if rawResults, ok := env.GetWorkingValue("rewoo.tool_results"); ok {
					results, _ = rawResults.([]RewooStepResult)
				}
				if ref := s.persistSynthesisArtifact(ctx, checkpointID, workflowID, runID, synthesis, results); ref != nil {
					env.SetWorkingValue("rewoo.synthesis_ref", *ref, contextdata.MemoryClassTask)
				}
			}
		}
	}
}

func envGetString(env *contextdata.Envelope, key string) string {
	val, _ := env.GetWorkingValue(key)
	if s, ok := val.(string); ok {
		return s
	}
	return ""
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
	record := agentlifecycle.WorkflowArtifactRecord{
		ArtifactID:        checkpointID + ".plan",
		WorkflowID:        workflowID,
		RunID:             runID,
		Kind:              "rewoo_plan",
		ContentType:       "application/json",
		StorageKind:       agentlifecycle.ArtifactStorageInline,
		SummaryText:       strings.TrimSpace(plan.Goal),
		SummaryMetadata:   map[string]any{"checkpoint_id": checkpointID, "agent": "rewoo"},
		InlineRawText:     string(payload),
		RawSizeBytes:      int64(len(payload)),
		CompressionMethod: "none",
		CreatedAt:         time.Now().UTC(),
	}
	if err := s.lifecycleRepo.UpsertArtifact(ctx, record); err != nil {
		return nil
	}
	// Phase 1 stub: return nil instead of artifact reference
	// TODO: Restore in Phase 8 when workflowutil is rewritten
	return nil
}

func (s *RewooCheckpointStore) persistToolResultsArtifact(ctx context.Context, checkpointID, workflowID, runID string, results []RewooStepResult) *core.ArtifactReference {
	payload, err := json.Marshal(results)
	if err != nil {
		return nil
	}
	record := agentlifecycle.WorkflowArtifactRecord{
		ArtifactID:        checkpointID + ".tool_results",
		WorkflowID:        workflowID,
		RunID:             runID,
		Kind:              "rewoo_tool_results",
		ContentType:       "application/json",
		StorageKind:       agentlifecycle.ArtifactStorageInline,
		SummaryText:       summarizeRewooStepResults(results),
		SummaryMetadata:   map[string]any{"checkpoint_id": checkpointID, "agent": "rewoo", "result_count": len(results)},
		InlineRawText:     string(payload),
		RawSizeBytes:      int64(len(payload)),
		CompressionMethod: "none",
		CreatedAt:         time.Now().UTC(),
	}
	if err := s.lifecycleRepo.UpsertArtifact(ctx, record); err != nil {
		return nil
	}
	// Phase 1 stub: return nil instead of artifact reference
	// TODO: Restore in Phase 8 when workflowutil is rewritten
	return nil
}

func (s *RewooCheckpointStore) persistSynthesisArtifact(ctx context.Context, checkpointID, workflowID, runID, synthesis string, results []RewooStepResult) *core.ArtifactReference {
	payload, err := json.Marshal(map[string]any{
		"synthesis":    synthesis,
		"step_results": results,
	})
	if err != nil {
		return nil
	}
	record := agentlifecycle.WorkflowArtifactRecord{
		ArtifactID:        checkpointID + ".synthesis",
		WorkflowID:        workflowID,
		RunID:             runID,
		Kind:              "rewoo_synthesis",
		ContentType:       "application/json",
		StorageKind:       agentlifecycle.ArtifactStorageInline,
		SummaryText:       synthesis,
		SummaryMetadata:   map[string]any{"checkpoint_id": checkpointID, "agent": "rewoo"},
		InlineRawText:     string(payload),
		RawSizeBytes:      int64(len(payload)),
		CompressionMethod: "none",
		CreatedAt:         time.Now().UTC(),
	}
	if err := s.lifecycleRepo.UpsertArtifact(ctx, record); err != nil {
		return nil
	}
	// Phase 1 stub: return nil instead of artifact reference
	// TODO: Restore in Phase 8 when workflowutil is rewritten
	return nil
}

func mustJSON(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}

func (s *RewooCheckpointStore) loadWorkflowArtifactJSON(ctx context.Context, ref core.ArtifactReference, target any) error {
	if s == nil || s.lifecycleRepo == nil {
		return fmt.Errorf("checkpoint_restore: lifecycle repository unavailable for artifact hydration")
	}
	artifacts, err := s.lifecycleRepo.ListArtifactsByRun(ctx, ref.RunID)
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
