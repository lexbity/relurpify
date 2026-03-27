package execution_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	"github.com/lexcodex/relurpify/archaeo/execution"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/stretchr/testify/require"
)

func TestEvaluateMutationsIgnoresInformationalMutation(t *testing.T) {
	ctx := context.Background()
	store := newMutationWorkflowStore(t, "wf-mutation-info")
	now := time.Date(2026, 3, 27, 7, 0, 0, 0, time.UTC)
	require.NoError(t, archaeoevents.AppendMutationEvent(ctx, store, archaeodomain.MutationEvent{
		WorkflowID:  "wf-mutation-info",
		PlanID:      "plan-1",
		Category:    archaeodomain.MutationObservation,
		SourceKind:  "learning_interaction",
		SourceRef:   "learn-1",
		Description: "supplemental note",
		BlastRadius: archaeodomain.BlastRadius{Scope: archaeodomain.BlastRadiusPlan},
		Impact:      archaeodomain.ImpactInformational,
		Disposition: archaeodomain.DispositionContinue,
		CreatedAt:   now,
	}))

	svc := execution.Service{WorkflowStore: store}
	plan := &frameworkplan.LivingPlan{ID: "plan-1", WorkflowID: "wf-mutation-info", Version: 1}
	step := &frameworkplan.PlanStep{ID: "step-1"}
	eval, err := svc.EvaluateMutations(ctx, "wf-mutation-info", nil, plan, step)
	require.NoError(t, err)
	require.NotNil(t, eval)
	require.False(t, eval.Blocking)
	require.Equal(t, archaeodomain.DispositionContinue, eval.Disposition)
}

func TestEvaluateMutationsBlocksActiveStepInvalidation(t *testing.T) {
	ctx := context.Background()
	store := newMutationWorkflowStore(t, "wf-mutation-step")
	now := time.Date(2026, 3, 27, 7, 5, 0, 0, time.UTC)
	require.NoError(t, archaeoevents.AppendMutationEvent(ctx, store, archaeodomain.MutationEvent{
		WorkflowID:  "wf-mutation-step",
		PlanID:      "plan-1",
		PlanVersion: intPtr(1),
		Category:    archaeodomain.MutationStepInvalidation,
		SourceKind:  "tension",
		SourceRef:   "tension-1",
		Description: "active step invalidated",
		BlastRadius: archaeodomain.BlastRadius{
			Scope:           archaeodomain.BlastRadiusStep,
			AffectedStepIDs: []string{"step-1"},
		},
		Impact:      archaeodomain.ImpactLocalBlocking,
		Disposition: archaeodomain.DispositionInvalidateStep,
		Blocking:    true,
		CreatedAt:   now,
	}))

	svc := execution.Service{WorkflowStore: store}
	plan := &frameworkplan.LivingPlan{ID: "plan-1", WorkflowID: "wf-mutation-step", Version: 1}
	step := &frameworkplan.PlanStep{ID: "step-1"}
	eval, err := svc.EvaluateMutations(ctx, "wf-mutation-step", nil, plan, step)
	require.NoError(t, err)
	require.NotNil(t, eval)
	require.True(t, eval.Blocking)
	require.Equal(t, archaeodomain.DispositionInvalidateStep, eval.Disposition)
}

func TestPreflightCoordinatorBlocksOnMutationDisposition(t *testing.T) {
	ctx := context.Background()
	store := newMutationWorkflowStore(t, "wf-mutation-preflight")
	now := time.Date(2026, 3, 27, 7, 10, 0, 0, time.UTC)
	require.NoError(t, archaeoevents.AppendMutationEvent(ctx, store, archaeodomain.MutationEvent{
		WorkflowID:  "wf-mutation-preflight",
		PlanID:      "plan-1",
		PlanVersion: intPtr(1),
		Category:    archaeodomain.MutationStepInvalidation,
		SourceKind:  "tension",
		SourceRef:   "tension-1",
		Description: "active step invalidated",
		BlastRadius: archaeodomain.BlastRadius{
			Scope:           archaeodomain.BlastRadiusStep,
			AffectedStepIDs: []string{"step-1"},
		},
		Impact:      archaeodomain.ImpactLocalBlocking,
		Disposition: archaeodomain.DispositionInvalidateStep,
		Blocking:    true,
		CreatedAt:   now,
	}))

	coord := execution.PreflightCoordinator{
		Service: execution.Service{
			WorkflowStore: store,
			Now:           func() time.Time { return now },
		},
		Plans: archaeoplans.Service{Store: preflightPlanStore{}, Now: func() time.Time { return now }},
	}
	plan := &frameworkplan.LivingPlan{
		ID:         "plan-1",
		WorkflowID: "wf-mutation-preflight",
		Version:    1,
		Steps: map[string]*frameworkplan.PlanStep{
			"step-1": {
				ID:        "step-1",
				Status:    frameworkplan.PlanStepPending,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}
	state := core.NewContext()
	outcome, err := coord.EvaluatePlanStepGate(ctx, &core.Task{Context: map[string]any{"workflow_id": "wf-mutation-preflight"}}, state, plan, plan.Steps["step-1"], nil)
	require.Error(t, err)
	require.True(t, outcome.ShouldInvalidate)
	require.NotNil(t, outcome.MutationEvaluation)
	require.NotNil(t, outcome.MutationCheckpoint)
	require.Equal(t, archaeodomain.MutationCheckpointPreExecution, outcome.MutationCheckpoint.Checkpoint)
	require.Equal(t, archaeodomain.DispositionInvalidateStep, outcome.MutationEvaluation.Disposition)
	raw, ok := state.Get("euclo.execution_mutation_checkpoints")
	require.True(t, ok)
	history, ok := raw.([]archaeodomain.MutationCheckpointSummary)
	require.True(t, ok)
	require.Len(t, history, 1)
	require.Equal(t, archaeodomain.MutationCheckpointPreExecution, history[0].Checkpoint)
}

func newMutationWorkflowStore(t *testing.T, workflowID string) *memorydb.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	require.NoError(t, store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  workflowID,
		TaskID:      "task-" + workflowID,
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "mutation workflow",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	return store
}

func intPtr(v int) *int { return &v }
