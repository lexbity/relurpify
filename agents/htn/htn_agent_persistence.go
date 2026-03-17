package htn

import (
	"context"
	"fmt"

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
func (a *HTNAgent) persistHTNRunSummary(ctx context.Context, store memory.WorkflowStateStore, workflowID, runID string) error {
	if store == nil || workflowID == "" || runID == "" {
		return nil
	}
	return persistence.SaveRunSummary(ctx, store, workflowID, runID)
}

// persistHTNMethodMetadata saves method metadata for future optimization.
func (a *HTNAgent) persistHTNMethodMetadata(ctx context.Context, store memory.WorkflowStateStore, workflowID string) error {
	if store == nil || workflowID == "" {
		return nil
	}
	return persistence.SaveMethodMetadata(ctx, store, workflowID)
}

// persistHTNExecutionMetrics saves detailed execution metrics.
func (a *HTNAgent) persistHTNExecutionMetrics(ctx context.Context, store memory.WorkflowStateStore, workflowID, runID string) error {
	if store == nil || workflowID == "" || runID == "" {
		return nil
	}
	return persistence.SaveExecutionMetrics(ctx, store, workflowID, runID)
}
