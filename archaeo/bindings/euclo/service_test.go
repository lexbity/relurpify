package euclobindings_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	euclobindings "github.com/lexcodex/relurpify/archaeo/bindings/euclo"
	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	"github.com/lexcodex/relurpify/archaeo/execution"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/stretchr/testify/require"
)

func TestEvaluateExecutionMutationsInformationalDoesNotBlock(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t, "wf-binding-info")
	now := time.Date(2026, 3, 27, 8, 0, 0, 0, time.UTC)
	require.NoError(t, archaeoevents.AppendMutationEvent(ctx, store, archaeodomain.MutationEvent{
		WorkflowID:  "wf-binding-info",
		PlanID:      "plan-1",
		Category:    archaeodomain.MutationObservation,
		SourceKind:  "learning_interaction",
		SourceRef:   "learn-1",
		Description: "observation",
		BlastRadius: archaeodomain.BlastRadius{Scope: archaeodomain.BlastRadiusPlan},
		Impact:      archaeodomain.ImpactInformational,
		Disposition: archaeodomain.DispositionContinue,
		CreatedAt:   now,
	}))

	binding := euclobindings.Runtime{WorkflowStore: store}
	eval, err := binding.EvaluateExecutionMutations(ctx, "wf-binding-info", nil, &frameworkplan.LivingPlan{ID: "plan-1", WorkflowID: "wf-binding-info", Version: 1}, &frameworkplan.PlanStep{ID: "step-1"})
	require.NoError(t, err)
	require.NotNil(t, eval)
	require.False(t, eval.Blocking)
	require.Equal(t, archaeodomain.DispositionContinue, eval.Disposition)
}

func TestEvaluateExecutionMutationsPlanStalenessHonorsPolicy(t *testing.T) {
	ctx := context.Background()
	store := newWorkflowStore(t, "wf-binding-stale")
	now := time.Date(2026, 3, 27, 8, 5, 0, 0, time.UTC)
	require.NoError(t, archaeoevents.AppendMutationEvent(ctx, store, archaeodomain.MutationEvent{
		WorkflowID:  "wf-binding-stale",
		PlanID:      "plan-1",
		PlanVersion: intPtr(2),
		Category:    archaeodomain.MutationPlanStaleness,
		SourceKind:  "plan_version",
		SourceRef:   "plan-1:2",
		Description: "plan stale",
		BlastRadius: archaeodomain.BlastRadius{Scope: archaeodomain.BlastRadiusPlan},
		Impact:      archaeodomain.ImpactPlanRecomputeRequired,
		Disposition: archaeodomain.DispositionContinueOnStalePlan,
		CreatedAt:   now,
	}))

	plan := &frameworkplan.LivingPlan{ID: "plan-1", WorkflowID: "wf-binding-stale", Version: 2}
	handoff := &archaeodomain.ExecutionHandoff{WorkflowID: "wf-binding-stale", PlanID: "plan-1", PlanVersion: 2, CreatedAt: now.Add(-time.Minute)}

	continueBinding := euclobindings.Runtime{
		WorkflowStore:  store,
		MutationPolicy: execution.MutationPolicy{ContinueOnStalePlan: true},
	}
	eval, err := continueBinding.EvaluateExecutionMutations(ctx, "wf-binding-stale", handoff, plan, &frameworkplan.PlanStep{ID: "step-1"})
	require.NoError(t, err)
	require.NotNil(t, eval)
	require.Equal(t, archaeodomain.DispositionContinueOnStalePlan, eval.Disposition)
	require.True(t, eval.ContinueOnStale)

	replanBinding := euclobindings.Runtime{
		WorkflowStore:  store,
		MutationPolicy: execution.MutationPolicy{ContinueOnStalePlan: false},
	}
	eval, err = replanBinding.EvaluateExecutionMutations(ctx, "wf-binding-stale", handoff, plan, &frameworkplan.PlanStep{ID: "step-1"})
	require.NoError(t, err)
	require.NotNil(t, eval)
	require.Equal(t, archaeodomain.DispositionRequireReplan, eval.Disposition)
	require.True(t, eval.RequireReplan)
}

func newWorkflowStore(t *testing.T, workflowID string) *memorydb.SQLiteWorkflowStateStore {
	t.Helper()
	store, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(t.TempDir(), "workflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	require.NoError(t, store.CreateWorkflow(context.Background(), memory.WorkflowRecord{
		WorkflowID:  workflowID,
		TaskID:      "task-" + workflowID,
		TaskType:    core.TaskTypeCodeGeneration,
		Instruction: "binding workflow",
		Status:      memory.WorkflowRunStatusRunning,
	}))
	return store
}

func intPtr(v int) *int { return &v }
