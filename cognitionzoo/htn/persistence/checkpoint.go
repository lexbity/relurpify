package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/cognitionzoo/htn/runtime"
	relurpctx "codeburg.org/lexbit/relurpify/context"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	frameworkpersistence "codeburg.org/lexbit/relurpify/context/persistence"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
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
	ref, err := frameworkpersistence.SaveCheckpointArtifact(ctx, env, func(artifact contextports.WorkflowArtifactRecord) error {
		return repo.UpsertArtifact(ctx, agentlifecycle.WorkflowArtifactRecord{
			ArtifactID:      artifact.ArtifactID,
			WorkflowID:      artifact.WorkflowID,
			RunID:           artifact.RunID,
			ContentType:     artifact.ContentType,
			StorageKind:     agentlifecycle.ArtifactStorageKind(artifact.StorageKind),
			SummaryText:     artifact.Summary,
			SummaryMetadata: artifact.Metadata,
			CreatedAt:       artifact.CreatedAt,
		})
	}, frameworkpersistence.CheckpointSnapshot{
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
		env.SetWorkingValueWithClass(runtime.ContextKeyCheckpointRef, *ref, contextdata.MemoryClassTask)
		env.SetWorkingValueWithClass(runtime.ContextKeyCheckpointSummary, SummarizeHTNCheckpoint(snapshot), contextdata.MemoryClassTask)
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

	latestPort, err := frameworkpersistence.LoadLatestCheckpointArtifact(ctx, func(runID string) ([]contextports.WorkflowArtifactRecord, error) {
		list, err := repo.ListArtifactsByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		out := make([]contextports.WorkflowArtifactRecord, 0, len(list))
		for _, a := range list {
			out = append(out, contextports.WorkflowArtifactRecord{
				ArtifactID:  a.ArtifactID,
				WorkflowID:  a.WorkflowID,
				RunID:       a.RunID,
				ContentType: a.ContentType,
				StorageKind: string(a.StorageKind),
				Summary:     a.SummaryText,
				Metadata:    a.SummaryMetadata,
				CreatedAt:   a.CreatedAt,
			})
		}
		return out, nil
	}, runID)
	if err != nil {
		return fmt.Errorf("htn: failed to list checkpoint artifacts: %w", err)
	}
	if latestPort == nil {
		return nil // no checkpoint to restore
	}

	snapshot, err := DecodeHTNCheckpointSnapshot(latestPort.Metadata["inline_raw"].(string))
	if err != nil {
		return fmt.Errorf("htn: failed to decode checkpoint: %w", err)
	}

	if err := restoreSnapshotToContext(env, snapshot); err != nil {
		return fmt.Errorf("htn: failed to restore checkpoint state: %w", err)
	}

	env.SetWorkingValueWithClass(runtime.ContextKeyCheckpointRef, relurpctx.ArtifactReference{
		ArtifactID: latestPort.ArtifactID,
		WorkflowID: latestPort.WorkflowID,
		RunID:      latestPort.RunID,
	}, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass(runtime.ContextKeyCheckpointSummary, latestPort.Summary, contextdata.MemoryClassTask)
	runtime.PublishResumeState(env, latestPort.ArtifactID)
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
		runtime.PublishTaskState(env, &execution.Task{
			ID:          snapshot.Task.ID,
			Type:        string(snapshot.Task.Type),
			Instruction: snapshot.Task.Instruction,
			Metadata:    taskMetadataToAny(snapshot.Task.Metadata),
		})
	}

	// Restore selected method.
	if snapshot.Method.Name != "" {
		env.SetWorkingValueWithClass(runtime.ContextKeySelectedMethod, snapshot.Method, contextdata.MemoryClassTask)
		env.SetWorkingValueWithClass(runtime.ContextKnowledgeMethod, snapshot.Method.Name, contextdata.MemoryClassTask)
	}

	// Restore plan.
	if snapshot.Plan != nil {
		runtime.PublishPlanState(env, snapshot.Plan)
	}

	// Restore execution state.
	runtime.PublishExecutionState(env, snapshot.Execution)

	// Restore metrics.
	env.SetWorkingValueWithClass(runtime.ContextKeyMetrics, snapshot.Metrics, contextdata.MemoryClassTask)

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


func taskMetadataToAny(input map[string]string) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}



// marshalJSON encodes a value to JSON bytes.
func marshalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

// unmarshalJSON decodes JSON bytes into a target value.
func unmarshalJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
