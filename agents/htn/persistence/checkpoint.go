package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/agents/htn/runtime"
	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	frameworkpersistence "codeburg.org/lexbit/relurpify/framework/persistence"
)

// saveHTNCheckpoint persists the current HTN execution state as a workflow artifact.
// This captures method, plan, completed steps, termination status, and dispatch metadata
// in a framework-managed, versioned, and resumable form.
func SaveCheckpoint(ctx context.Context, env *contextdata.Envelope, repo agentlifecycle.Repository, workflowID, runID string) error {
	if env == nil || repo == nil || workflowID == "" || runID == "" {
		return nil // silently skip if preconditions unmet
	}

	snapshot, loaded, err := runtime.LoadStateFromEnvelope(env)
	if err != nil {
		return fmt.Errorf("htn: failed to load state for checkpoint: %w", err)
	}
	if !loaded || snapshot == nil {
		return nil // nothing to checkpoint
	}

	checkpointJSON, err := MarshalHTNCheckpointSnapshot(snapshot)
	if err != nil {
		return fmt.Errorf("htn: failed to encode checkpoint: %w", err)
	}

	artifactID := generateCheckpointID()
	ref, err := frameworkpersistence.SaveCheckpointArtifact(ctx, env, repo, frameworkpersistence.CheckpointSnapshot{
		CheckpointID: artifactID,
		WorkflowID:   workflowID,
		RunID:        runID,
		Kind:         "htn_checkpoint",
		Summary:      SummarizeHTNCheckpoint(snapshot),
		Metadata:     HTNCheckpointMetadata(snapshot),
		InlineRaw:    checkpointJSON,
	})
	if err != nil {
		return fmt.Errorf("htn: failed to save checkpoint artifact: %w", err)
	}
	if ref != nil {
		env.SetWorkingValue(runtime.ContextKeyCheckpointRef, *ref, contextdata.MemoryClassTask)
		env.SetWorkingValue(runtime.ContextKeyCheckpointSummary, SummarizeHTNCheckpoint(snapshot), contextdata.MemoryClassTask)
	}

	// Update execution state with checkpoint ID.
	execution := runtime.LoadExecutionState(env)
	execution.ResumeCheckpointID = artifactID
	runtime.PublishExecutionState(env, execution)

	return nil
}

// restoreHTNCheckpoint loads the latest HTN checkpoint from the lifecycle
// repository and restores method, plan, completed steps, termination status, and
// dispatch metadata.
func RestoreCheckpoint(ctx context.Context, env *contextdata.Envelope, repo agentlifecycle.Repository, workflowID, runID string) error {
	if env == nil || repo == nil || workflowID == "" || runID == "" {
		return nil
	}

	latestCheckpoint, err := frameworkpersistence.LoadLatestCheckpointArtifact(ctx, repo, runID, "htn_checkpoint")
	if err != nil {
		return fmt.Errorf("htn: failed to list checkpoint artifacts: %w", err)
	}
	if latestCheckpoint == nil {
		return nil // no checkpoint to restore
	}

	snapshot, err := DecodeHTNCheckpointSnapshot(latestCheckpoint.InlineRawText)
	if err != nil {
		return fmt.Errorf("htn: failed to decode checkpoint: %w", err)
	}

	if err := restoreSnapshotToContext(env, snapshot); err != nil {
		return fmt.Errorf("htn: failed to restore checkpoint state: %w", err)
	}

	env.SetWorkingValue(runtime.ContextKeyCheckpointRef, core.ArtifactReference{
		ArtifactID: latestCheckpoint.ArtifactID,
		WorkflowID: latestCheckpoint.WorkflowID,
		RunID:      latestCheckpoint.RunID,
	}, contextdata.MemoryClassTask)
	env.SetWorkingValue(runtime.ContextKeyCheckpointSummary, latestCheckpoint.SummaryText, contextdata.MemoryClassTask)
	runtime.PublishResumeState(env, latestCheckpoint.ArtifactID)
	return nil
}

// encodeSnapshot serializes HTN state to JSON.
// MarshalHTNCheckpointSnapshot serializes HTN state to JSON.
func MarshalHTNCheckpointSnapshot(snapshot *runtime.HTNState) (string, error) {
	data, err := marshalJSON(snapshot)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// DecodeHTNCheckpointSnapshot deserializes HTN state from JSON.
func DecodeHTNCheckpointSnapshot(jsonText string) (*runtime.HTNState, error) {
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

// restoreSnapshotToContext populates envelope with restored checkpoint data.
func restoreSnapshotToContext(env *contextdata.Envelope, snapshot *runtime.HTNState) error {
	if env == nil || snapshot == nil {
		return nil
	}

	// Restore task state.
	if snapshot.Task.ID != "" {
		runtime.PublishTaskState(env, &core.Task{
			ID:          snapshot.Task.ID,
			Type:        snapshot.Task.Type,
			Instruction: snapshot.Task.Instruction,
			Metadata:    runtime.MapsClone(snapshot.Task.Metadata),
		})
	}

	// Restore selected method.
	if snapshot.Method.Name != "" {
		env.SetWorkingValue(runtime.ContextKeySelectedMethod, snapshot.Method, contextdata.MemoryClassTask)
		env.SetWorkingValue(runtime.ContextKnowledgeMethod, snapshot.Method.Name, contextdata.MemoryClassTask)
	}

	// Restore plan.
	if snapshot.Plan != nil {
		runtime.PublishPlanState(env, snapshot.Plan)
	}

	// Restore execution state.
	runtime.PublishExecutionState(env, snapshot.Execution)

	// Restore metrics.
	env.SetWorkingValue(runtime.ContextKeyMetrics, snapshot.Metrics, contextdata.MemoryClassTask)

	// Restore preflight state.
	if snapshot.Preflight.Report != nil {
		runtime.PublishPreflightState(env, snapshot.Preflight.Report, nil)
	} else if snapshot.Preflight.Error != "" {
		err := fmt.Errorf("preflight error: %s", snapshot.Preflight.Error)
		runtime.PublishPreflightState(env, nil, err)
	}

	// Restore retrieval state.
	if snapshot.RetrievalApplied {
		runtime.PublishWorkflowRetrieval(env, nil, true)
	}

	// Restore termination.
	if snapshot.Termination != "" {
		runtime.PublishTerminationState(env, snapshot.Termination)
	}

	// Mark as resumed.
	runtime.PublishResumeState(env, snapshot.ResumeCheckpointID)

	// Final validation.
	if _, _, err := runtime.LoadStateFromEnvelope(env); err != nil {
		return fmt.Errorf("htn: restored state validation failed: %w", err)
	}

	return nil
}

// SummarizeHTNCheckpoint produces a human-readable checkpoint summary.
func SummarizeHTNCheckpoint(snapshot *runtime.HTNState) string {
	parts := []string{
		fmt.Sprintf("Task: %s (%s)", snapshot.Task.ID, snapshot.Task.Type),
		fmt.Sprintf("Method: %s", snapshot.Method.Name),
		fmt.Sprintf("Progress: %d/%d steps", snapshot.Execution.CompletedStepCount, snapshot.Execution.PlannedStepCount),
		fmt.Sprintf("Status: %s", snapshot.Termination),
	}
	return strings.Join(parts, " | ")
}

// HTNCheckpointMetadata constructs metadata for checkpoint tracking.
func HTNCheckpointMetadata(snapshot *runtime.HTNState) map[string]any {
	metadata := map[string]any{
		"schema_version":     runtime.HTNSchemaVersion,
		"task_type":          string(snapshot.Task.Type),
		"method_name":        snapshot.Method.Name,
		"planned_steps":      snapshot.Execution.PlannedStepCount,
		"completed_steps":    snapshot.Execution.CompletedStepCount,
		"termination_status": snapshot.Termination,
		"retrieval_applied":  snapshot.RetrievalApplied,
	}
	if len(snapshot.Execution.CompletedSteps) > 0 {
		metadata["last_completed_step"] = snapshot.Execution.LastCompletedStep
	}
	return metadata
}

// generateCheckpointID creates a unique checkpoint identifier.
func generateCheckpointID() string {
	return fmt.Sprintf("htn_checkpoint_%d", time.Now().UnixNano())
}

// persistDispatchMetadata saves the last dispatch decision to envelope for recovery purposes.
func persistDispatchMetadata(env *contextdata.Envelope, dispatcher string, target string, reason string) {
	if env == nil {
		return
	}
	metadata := map[string]any{
		"requested_target": target,
		"resolved_target":  target,
		"mode":             dispatcher,
		"reason":           reason,
		"timestamp":        time.Now().UTC().Unix(),
	}
	env.SetWorkingValue(runtime.ContextKeyCheckpoint, metadata, contextdata.MemoryClassTask)
}

// persistRecoveryMetadata saves recovery diagnosis to envelope for resume.
func persistRecoveryMetadata(env *contextdata.Envelope, diagnosis string, notes []string, stepID string, err error) {
	if env == nil {
		return
	}
	env.SetWorkingValue(runtime.ContextKeyLastRecoveryDiag, diagnosis, contextdata.MemoryClassTask)
	env.SetWorkingValue(runtime.ContextKeyLastRecoveryNotes, append([]string{}, notes...), contextdata.MemoryClassTask)
	env.SetWorkingValue(runtime.ContextKeyLastFailureStep, stepID, contextdata.MemoryClassTask)
	if err != nil {
		env.SetWorkingValue(runtime.ContextKeyLastFailureError, err.Error(), contextdata.MemoryClassTask)
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
