package htn

import (
	"context"
	"time"

	"github.com/lexcodex/relurpify/agents/htn/persistence"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
)

// Persistence-related methods for HTNAgent (moved from persistence subpackage to allow method definitions)

// saveHTNCheckpoint persists the current HTN execution state as a workflow artifact.
func (a *HTNAgent) saveHTNCheckpoint(ctx context.Context, state *core.Context, store memory.WorkflowStateStore, workflowID, runID string) error {
	if state == nil || store == nil || workflowID == "" || runID == "" {
		return nil
	}
	return persistence.SaveCheckpoint(ctx, state, store, workflowID, runID)
}

// restoreHTNCheckpoint restores HTN state from a checkpoint.
func (a *HTNAgent) restoreHTNCheckpoint(ctx context.Context, state *core.Context, store memory.WorkflowStateStore, workflowID, runID string) error {
	if state == nil || store == nil || workflowID == "" || runID == "" {
		return nil
	}
	return persistence.RestoreCheckpoint(ctx, state, store, workflowID, runID)
}

// persistHTNRunSummary saves metrics and execution metadata.
func (a *HTNAgent) persistHTNRunSummary(ctx context.Context, state *core.Context, store memory.WorkflowStateStore, workflowID, runID string, startTime time.Time, success bool, err error) error {
	if store == nil || workflowID == "" || runID == "" {
		return nil
	}
	return persistence.SaveRunSummary(ctx, state, store, workflowID, runID, startTime, success, err)
}

// persistHTNMethodMetadata saves method metadata for future optimization.
func (a *HTNAgent) persistHTNMethodMetadata(ctx context.Context, state *core.Context, store memory.WorkflowStateStore, workflowID, runID string) error {
	if store == nil || workflowID == "" || runID == "" {
		return nil
	}
	return persistence.SaveMethodMetadata(ctx, state, store, workflowID, runID)
}

// persistHTNExecutionMetrics saves detailed execution metrics.
func (a *HTNAgent) persistHTNExecutionMetrics(ctx context.Context, state *core.Context, store memory.WorkflowStateStore, workflowID, runID string, decompositionTime, executionTime time.Duration) error {
	if store == nil || workflowID == "" || runID == "" {
		return nil
	}
	return persistence.SaveExecutionMetrics(ctx, state, store, workflowID, runID, decompositionTime, executionTime)
}

// persistOperatorOutcome records individual operator step outcomes.
func (a *HTNAgent) persistOperatorOutcome(ctx context.Context, store memory.WorkflowStateStore, workflowID, runID, stepRunID, operator, stepID string, duration int, success bool, outputKeys []string, err error) error {
	if store == nil || workflowID == "" || runID == "" {
		return nil
	}
	return persistence.PersistOperatorOutcome(ctx, store, workflowID, runID, stepRunID, operator, stepID, time.Duration(duration)*time.Second, success, outputKeys, err)
}
