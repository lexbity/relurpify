package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/agents/htn/runtime"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
)

// checkpointPersistence handles HTN-specific checkpoint recording to the workflow
// state store. This unifies HTN's typed state snapshots with framework-owned
// persistence semantics.
type checkpointPersistence struct {
	store      memory.WorkflowStateStore
	workflowID string
	runID      string
	taskID     string
}

// saveHTNCheckpoint persists the current HTN execution state as a workflow artifact.
// This captures method, plan, completed steps, termination status, and dispatch metadata
// in a framework-managed, versioned, and resumable form.
func SaveCheckpoint(ctx context.Context, state *core.Context, store memory.WorkflowStateStore, workflowID, runID string) error {
	if state == nil || store == nil || workflowID == "" || runID == "" {
		return nil // silently skip if preconditions unmet
	}

	snapshot, loaded, err := runtime.LoadStateFromContext(state)
	if err != nil {
		return fmt.Errorf("htn: failed to load state for checkpoint: %w", err)
	}
	if !loaded || snapshot == nil {
		return nil // nothing to checkpoint
	}

	cp := checkpointPersistence{
		store:      store,
		workflowID: workflowID,
		runID:      runID,
		taskID:     snapshot.Task.ID,
	}

	// Serialize the HTN state snapshot.
	checkpointJSON, err := cp.encodeSnapshot(snapshot)
	if err != nil {
		return fmt.Errorf("htn: failed to encode checkpoint: %w", err)
	}

	// Persist as a workflow artifact.
	artifact := memory.WorkflowArtifactRecord{
		ArtifactID:        cp.generateCheckpointID(),
		WorkflowID:        workflowID,
		RunID:             runID,
		Kind:              "htn_checkpoint",
		ContentType:       "application/json",
		StorageKind:       memory.ArtifactStorageInline,
		SummaryText:       cp.summarizeCheckpoint(snapshot),
		SummaryMetadata:   cp.checkpointMetadata(snapshot),
		InlineRawText:     checkpointJSON,
		CompressionMethod: "",
		CreatedAt:         time.Now().UTC(),
	}

	if err := store.UpsertWorkflowArtifact(ctx, artifact); err != nil {
		return fmt.Errorf("htn: failed to save checkpoint artifact: %w", err)
	}

	// Update execution state with checkpoint ID.
	execution := runtime.LoadExecutionState(state)
	execution.ResumeCheckpointID = artifact.ArtifactID
	runtime.PublishExecutionState(state, execution)

	return nil
}

// restoreHTNCheckpoint loads the latest HTN checkpoint from the workflow state
// store and restores method, plan, completed steps, termination status, and
// dispatch metadata.
func RestoreCheckpoint(ctx context.Context, state *core.Context, store memory.WorkflowStateStore, workflowID, runID string) error {
	if state == nil || store == nil || workflowID == "" || runID == "" {
		return nil
	}

	// Load the latest checkpoint artifact.
	artifacts, err := store.ListWorkflowArtifacts(ctx, workflowID, runID)
	if err != nil {
		return fmt.Errorf("htn: failed to list checkpoint artifacts: %w", err)
	}

	var latestCheckpoint *memory.WorkflowArtifactRecord
	for i := range artifacts {
		if strings.TrimSpace(artifacts[i].Kind) == "htn_checkpoint" {
			latestCheckpoint = &artifacts[i]
			break // artifacts are ordered newest-first
		}
	}

	if latestCheckpoint == nil {
		return nil // no checkpoint to restore
	}

	cp := checkpointPersistence{
		store:      store,
		workflowID: workflowID,
		runID:      runID,
	}

	snapshot, err := cp.decodeSnapshot(latestCheckpoint.InlineRawText)
	if err != nil {
		return fmt.Errorf("htn: failed to decode checkpoint: %w", err)
	}

	if err := cp.restoreSnapshotToContext(state, snapshot); err != nil {
		return fmt.Errorf("htn: failed to restore checkpoint state: %w", err)
	}

	runtime.PublishResumeState(state, latestCheckpoint.ArtifactID)
	return nil
}

// encodeSnapshot serializes HTN state to JSON.
func (cp *checkpointPersistence) encodeSnapshot(snapshot *runtime.HTNState) (string, error) {
	data, err := marshalJSON(snapshot)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// decodeSnapshot deserializes HTN state from JSON.
func (cp *checkpointPersistence) decodeSnapshot(jsonText string) (*runtime.HTNState, error) {
	var snapshot runtime.HTNState
	if err := unmarshalJSON([]byte(jsonText), &snapshot); err != nil {
		return nil, err
	}
	runtime.NormalizeHTNState(&snapshot)
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// restoreSnapshotToContext populates state with restored checkpoint data.
func (cp *checkpointPersistence) restoreSnapshotToContext(state *core.Context, snapshot *runtime.HTNState) error {
	if state == nil || snapshot == nil {
		return nil
	}

	// Restore task state.
	if snapshot.Task.ID != "" {
		runtime.PublishTaskState(state, &core.Task{
			ID:          snapshot.Task.ID,
			Type:        snapshot.Task.Type,
			Instruction: snapshot.Task.Instruction,
			Metadata:    runtime.MapsClone(snapshot.Task.Metadata),
		})
	}

	// Restore selected method.
	if snapshot.Method.Name != "" {
		state.Set(runtime.ContextKeySelectedMethod, snapshot.Method)
		state.SetKnowledge(runtime.ContextKnowledgeMethod, snapshot.Method.Name)
	}

	// Restore plan.
	if snapshot.Plan != nil {
		runtime.PublishPlanState(state, snapshot.Plan)
	}

	// Restore execution state.
	runtime.PublishExecutionState(state, snapshot.Execution)

	// Restore metrics.
	state.Set(runtime.ContextKeyMetrics, snapshot.Metrics)

	// Restore preflight state.
	if snapshot.Preflight.Report != nil {
		runtime.PublishPreflightState(state, snapshot.Preflight.Report, nil)
	} else if snapshot.Preflight.Error != "" {
		err := fmt.Errorf("preflight error: %s", snapshot.Preflight.Error)
		runtime.PublishPreflightState(state, nil, err)
	}

	// Restore retrieval state.
	if snapshot.RetrievalApplied {
		runtime.PublishWorkflowRetrieval(state, nil, true)
	}

	// Restore termination.
	if snapshot.Termination != "" {
		runtime.PublishTerminationState(state, snapshot.Termination)
	}

	// Mark as resumed.
	runtime.PublishResumeState(state, snapshot.ResumeCheckpointID)

	// Final validation.
	if _, _, err := runtime.LoadStateFromContext(state); err != nil {
		return fmt.Errorf("htn: restored state validation failed: %w", err)
	}

	return nil
}

// summarizeCheckpoint produces a human-readable checkpoint summary.
func (cp *checkpointPersistence) summarizeCheckpoint(snapshot *runtime.HTNState) string {
	parts := []string{
		fmt.Sprintf("Task: %s (%s)", snapshot.Task.ID, snapshot.Task.Type),
		fmt.Sprintf("Method: %s", snapshot.Method.Name),
		fmt.Sprintf("Progress: %d/%d steps", snapshot.Execution.CompletedStepCount, snapshot.Execution.PlannedStepCount),
		fmt.Sprintf("Status: %s", snapshot.Termination),
	}
	return strings.Join(parts, " | ")
}

// checkpointMetadata constructs metadata for checkpoint tracking.
func (cp *checkpointPersistence) checkpointMetadata(snapshot *runtime.HTNState) map[string]any {
	metadata := map[string]any{
		"schema_version":       runtime.HTNSchemaVersion,
		"task_type":            string(snapshot.Task.Type),
		"method_name":          snapshot.Method.Name,
		"planned_steps":        snapshot.Execution.PlannedStepCount,
		"completed_steps":      snapshot.Execution.CompletedStepCount,
		"termination_status":   snapshot.Termination,
		"retrieval_applied":    snapshot.RetrievalApplied,
	}
	if len(snapshot.Execution.CompletedSteps) > 0 {
		metadata["last_completed_step"] = snapshot.Execution.LastCompletedStep
	}
	return metadata
}

// generateCheckpointID creates a unique checkpoint identifier.
func (cp *checkpointPersistence) generateCheckpointID() string {
	return fmt.Sprintf("htn_checkpoint_%d", time.Now().UnixNano())
}

// persistDispatchMetadata saves the last dispatch decision to context for recovery purposes.
func persistDispatchMetadata(state *core.Context, dispatcher string, target string, reason string) {
	if state == nil {
		return
	}
	metadata := map[string]any{
		"requested_target": target,
		"resolved_target":  target,
		"mode":             dispatcher,
		"reason":           reason,
		"timestamp":        time.Now().UTC().Unix(),
	}
	state.Set(runtime.ContextKeyCheckpoint, metadata)
}

// persistRecoveryMetadata saves recovery diagnosis to context for resume.
func persistRecoveryMetadata(state *core.Context, diagnosis string, notes []string, stepID string, err error) {
	if state == nil {
		return
	}
	state.Set(runtime.ContextKeyLastRecoveryDiag, diagnosis)
	state.Set(runtime.ContextKeyLastRecoveryNotes, append([]string{}, notes...))
	state.Set(runtime.ContextKeyLastFailureStep, stepID)
	if err != nil {
		state.Set(runtime.ContextKeyLastFailureError, err.Error())
	}
}

// marshalJSON encodes a value to JSON bytes.
func marshalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

// unmarshalJSON decodes JSON bytes into a target value.
func unmarshalJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
