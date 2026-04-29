package htn

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/agents/htn/persistence"
	"codeburg.org/lexbit/relurpify/agents/htn/runtime"
	"codeburg.org/lexbit/relurpify/framework/agentlifecycle"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	frameworkpersistence "codeburg.org/lexbit/relurpify/framework/persistence"
)

// envelopeGet retrieves a value from envelope working memory.
func envelopeGet(state *contextdata.Envelope, key string) (any, bool) {
	if state == nil {
		return nil, false
	}
	return state.GetWorkingValue(key)
}

// envelopeSet stores a value in envelope working memory with task scope.
func envelopeSet(state *contextdata.Envelope, key string, value any) {
	if state == nil {
		return
	}
	state.SetWorkingValue(key, value, contextdata.MemoryClassTask)
}

// Persistence-related methods for HTNAgent (moved from persistence subpackage to allow method definitions)

// saveHTNCheckpoint persists the current HTN execution state as a workflow artifact.
func (a *HTNAgent) saveHTNCheckpoint(ctx context.Context, state *contextdata.Envelope, repo agentlifecycle.Repository, workflowID, runID string) error {
	if state == nil || repo == nil || workflowID == "" || runID == "" {
		return nil
	}
	snapshot, loaded, err := runtime.LoadStateFromEnvelope(state)
	if err != nil {
		return err
	}
	if !loaded || snapshot == nil {
		return nil
	}
	payload, err := persistence.MarshalHTNCheckpointSnapshot(snapshot)
	if err != nil {
		return err
	}
	_, err = frameworkpersistence.SaveCheckpointArtifact(ctx, state, repo, frameworkpersistence.CheckpointSnapshot{
		CheckpointID: snapshot.ResumeCheckpointID,
		WorkflowID:   workflowID,
		RunID:        runID,
		Kind:         "htn_checkpoint",
		Summary:      persistence.SummarizeHTNCheckpoint(snapshot),
		Metadata:     persistence.HTNCheckpointMetadata(snapshot),
		InlineRaw:    payload,
	})
	return err
}

// restoreHTNCheckpoint restores HTN state from a checkpoint.
func (a *HTNAgent) restoreHTNCheckpoint(ctx context.Context, state *contextdata.Envelope, repo agentlifecycle.Repository, workflowID, runID string) error {
	if state == nil || repo == nil || workflowID == "" || runID == "" {
		return nil
	}
	return persistence.RestoreCheckpoint(ctx, state, repo, workflowID, runID)
}

// persistHTNRunSummary saves metrics and execution metadata.
func (a *HTNAgent) persistHTNRunSummary(ctx context.Context, state *contextdata.Envelope, repo agentlifecycle.Repository, workflowID, runID string, startTime time.Time, success bool, err error) error {
	if repo == nil || workflowID == "" || runID == "" {
		return nil
	}
	return persistence.SaveRunSummary(ctx, state, repo, workflowID, runID, startTime, success, err)
}

// persistHTNMethodMetadata saves method metadata for future optimization.
func (a *HTNAgent) persistHTNMethodMetadata(ctx context.Context, state *contextdata.Envelope, repo agentlifecycle.Repository, workflowID, runID string) error {
	if repo == nil || workflowID == "" || runID == "" {
		return nil
	}
	return persistence.SaveMethodMetadata(ctx, state, repo, workflowID, runID)
}

// persistHTNExecutionMetrics saves detailed execution metrics.
func (a *HTNAgent) persistHTNExecutionMetrics(ctx context.Context, state *contextdata.Envelope, repo agentlifecycle.Repository, workflowID, runID string, decompositionTime, executionTime time.Duration) error {
	if repo == nil || workflowID == "" || runID == "" {
		return nil
	}
	return persistence.SaveExecutionMetrics(ctx, state, repo, workflowID, runID, decompositionTime, executionTime)
}

// persistOperatorOutcome records individual operator step outcomes.
func (a *HTNAgent) persistOperatorOutcome(ctx context.Context, repo agentlifecycle.Repository, workflowID, runID, stepRunID, operator, stepID string, duration int, success bool, outputKeys []string, err error) error {
	if repo == nil || workflowID == "" || runID == "" {
		return nil
	}
	return persistence.PersistOperatorOutcome(ctx, repo, workflowID, runID, stepRunID, operator, stepID, time.Duration(duration)*time.Second, success, outputKeys, err)
}

// compactHTNCheckpointState replaces the full checkpoint in state with a
// lightweight summary map. Only runs when a checkpoint artifact ref is present,
// indicating a full checkpoint was already persisted externally.
func compactHTNCheckpointState(state *contextdata.Envelope) {
	if state == nil {
		return
	}
	if _, ok := envelopeGet(state, runtime.ContextKeyCheckpointRef); !ok {
		return
	}
	raw, ok := envelopeGet(state, runtime.ContextKeyCheckpoint)
	if !ok {
		return
	}
	switch checkpoint := raw.(type) {
	case runtime.CheckpointState:
		envelopeSet(state, runtime.ContextKeyCheckpoint, compactHTNCheckpoint(checkpoint))
	case *runtime.CheckpointState:
		if checkpoint != nil {
			envelopeSet(state, runtime.ContextKeyCheckpoint, compactHTNCheckpoint(*checkpoint))
		}
	case map[string]any:
		envelopeSet(state, runtime.ContextKeyCheckpoint, compactHTNCheckpointMap(checkpoint))
	}
}

func compactHTNCheckpoint(checkpoint runtime.CheckpointState) map[string]any {
	return map[string]any{
		"checkpoint_id":   checkpoint.CheckpointID,
		"stage_name":      checkpoint.StageName,
		"stage_index":     checkpoint.StageIndex,
		"workflow_id":     checkpoint.WorkflowID,
		"run_id":          checkpoint.RunID,
		"completed_steps": len(checkpoint.CompletedSteps),
		"has_snapshot":    checkpoint.Snapshot != nil,
		"schema_version":  checkpoint.SchemaVersion,
	}
}

func compactHTNCheckpointMap(checkpoint map[string]any) map[string]any {
	value := map[string]any{
		"checkpoint_id":  checkpoint["checkpoint_id"],
		"stage_name":     checkpoint["stage_name"],
		"stage_index":    checkpoint["stage_index"],
		"workflow_id":    checkpoint["workflow_id"],
		"run_id":         checkpoint["run_id"],
		"schema_version": checkpoint["schema_version"],
	}
	if completed, ok := checkpoint["completed_steps"]; ok {
		switch values := completed.(type) {
		case []string:
			value["completed_steps"] = len(values)
		case []any:
			value["completed_steps"] = len(values)
		}
	}
	_, hasSnapshot := checkpoint["snapshot"]
	value["has_snapshot"] = hasSnapshot
	return value
}
