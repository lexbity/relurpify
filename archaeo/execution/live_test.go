package execution_test

import (
	"context"
	"testing"
	"time"

	archaeodomain "github.com/lexcodex/relurpify/archaeo/domain"
	archaeoevents "github.com/lexcodex/relurpify/archaeo/events"
	memorydb "github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/archaeo/execution"
	archaeoplans "github.com/lexcodex/relurpify/archaeo/plans"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/stretchr/testify/require"
)

func TestLiveMutationCoordinatorContinuesOnObservation(t *testing.T) {
	ctx := context.Background()
	store := newMutationWorkflowStore(t, "wf-live-observation")
	now := time.Date(2026, 3, 27, 9, 0, 0, 0, time.UTC)
	require.NoError(t, archaeoevents.AppendMutationEvent(ctx, store, archaeodomain.MutationEvent{
		WorkflowID:  "wf-live-observation",
		PlanID:      "plan-1",
		Category:    archaeodomain.MutationObservation,
		SourceKind:  "comment",
		SourceRef:   "comment-1",
		Description: "supplemental note",
		BlastRadius: archaeodomain.BlastRadius{Scope: archaeodomain.BlastRadiusPlan},
		Impact:      archaeodomain.ImpactInformational,
		Disposition: archaeodomain.DispositionContinue,
		CreatedAt:   now,
	}))
	plan := testPlan("wf-live-observation", now)
	step := plan.Steps["step-1"]
	state := core.NewContext()
	state.Set("euclo.execution_handoff", archaeodomain.ExecutionHandoff{
		WorkflowID:  "wf-live-observation",
		PlanID:      "plan-1",
		PlanVersion: 1,
		CreatedAt:   now.Add(-time.Minute),
	})
	coord := execution.LiveMutationCoordinator{
		Service: execution.Service{WorkflowStore: store},
		Plans:   archaeoplans.Service{Store: preflightPlanStore{}, Now: func() time.Time { return now }},
	}

	eval, err := coord.CheckpointExecution(ctx, &core.Task{Context: map[string]any{"workflow_id": "wf-live-observation"}}, state, plan, step)
	require.NoError(t, err)
	require.NotNil(t, eval)
	require.Equal(t, archaeodomain.DispositionContinue, eval.Disposition)
	require.Equal(t, archaeodomain.DispositionContinue, archaeodomain.ExecutionDisposition(state.GetString("euclo.execution_mutation_disposition")))
	raw, ok := state.Get("euclo.execution_mutation_checkpoints")
	require.True(t, ok)
	history, ok := raw.([]archaeodomain.MutationCheckpointSummary)
	require.True(t, ok)
	require.Len(t, history, 1)
	require.Equal(t, archaeodomain.MutationCheckpointPreVerification, history[0].Checkpoint)
}

func TestLiveMutationCoordinatorBlocksActiveStepTension(t *testing.T) {
	ctx := context.Background()
	store := newMutationWorkflowStore(t, "wf-live-block")
	now := time.Date(2026, 3, 27, 9, 5, 0, 0, time.UTC)
	require.NoError(t, archaeoevents.AppendMutationEvent(ctx, store, archaeodomain.MutationEvent{
		WorkflowID:  "wf-live-block",
		PlanID:      "plan-1",
		PlanVersion: intPtr(1),
		Category:    archaeodomain.MutationBlockingSemantic,
		SourceKind:  "tension",
		SourceRef:   "tension-1",
		Description: "critical contradiction on active step",
		BlastRadius: archaeodomain.BlastRadius{
			Scope:           archaeodomain.BlastRadiusStep,
			AffectedStepIDs: []string{"step-1"},
			EstimatedCount:  1,
		},
		Impact:      archaeodomain.ImpactLocalBlocking,
		Disposition: archaeodomain.DispositionBlockExecution,
		Blocking:    true,
		CreatedAt:   now,
	}))
	plan := testPlan("wf-live-block", now)
	step := plan.Steps["step-1"]
	state := core.NewContext()
	state.Set("euclo.execution_handoff", archaeodomain.ExecutionHandoff{
		WorkflowID:  "wf-live-block",
		PlanID:      "plan-1",
		PlanVersion: 1,
		CreatedAt:   now.Add(-time.Minute),
	})
	coord := execution.LiveMutationCoordinator{
		Service: execution.Service{WorkflowStore: store},
		Plans:   archaeoplans.Service{Store: preflightPlanStore{}, Now: func() time.Time { return now }},
	}

	eval, err := coord.CheckpointExecution(ctx, &core.Task{Context: map[string]any{"workflow_id": "wf-live-block"}}, state, plan, step)
	require.Error(t, err)
	require.NotNil(t, eval)
	require.Equal(t, archaeodomain.DispositionBlockExecution, eval.Disposition)
	require.Equal(t, frameworkplan.PlanStepPending, step.Status)
	require.Equal(t, archaeodomain.DispositionBlockExecution, archaeodomain.ExecutionDisposition(state.GetString("euclo.execution_mutation_disposition")))
}

func TestLiveMutationCoordinatorHonorsContinueOnStalePolicy(t *testing.T) {
	ctx := context.Background()
	store := newMutationWorkflowStore(t, "wf-live-stale")
	now := time.Date(2026, 3, 27, 9, 10, 0, 0, time.UTC)
	require.NoError(t, archaeoevents.AppendMutationEvent(ctx, store, archaeodomain.MutationEvent{
		WorkflowID:  "wf-live-stale",
		PlanID:      "plan-1",
		PlanVersion: intPtr(1),
		Category:    archaeodomain.MutationPlanStaleness,
		SourceKind:  "plan_version",
		SourceRef:   "plan-1:1",
		Description: "plan became stale",
		BlastRadius: archaeodomain.BlastRadius{Scope: archaeodomain.BlastRadiusPlan, EstimatedCount: 1},
		Impact:      archaeodomain.ImpactPlanRecomputeRequired,
		Disposition: archaeodomain.DispositionRequireReplan,
		CreatedAt:   now,
	}))
	plan := testPlan("wf-live-stale", now)
	step := plan.Steps["step-1"]
	state := core.NewContext()
	state.Set("euclo.execution_handoff", archaeodomain.ExecutionHandoff{
		WorkflowID:  "wf-live-stale",
		PlanID:      "plan-1",
		PlanVersion: 1,
		CreatedAt:   now.Add(-time.Minute),
	})
	coord := execution.LiveMutationCoordinator{
		Service: execution.Service{
			WorkflowStore:  store,
			MutationPolicy: execution.MutationPolicy{ContinueOnStalePlan: true},
		},
		Plans: archaeoplans.Service{Store: preflightPlanStore{}, Now: func() time.Time { return now }},
	}

	eval, err := coord.CheckpointExecution(ctx, &core.Task{Context: map[string]any{"workflow_id": "wf-live-stale"}}, state, plan, step)
	require.NoError(t, err)
	require.NotNil(t, eval)
	require.Equal(t, archaeodomain.DispositionContinueOnStalePlan, eval.Disposition)
	require.Equal(t, "true", state.GetString("euclo.execution_on_stale_plan"))
	require.Equal(t, "false", state.GetString("euclo.execution_requires_replan"))
}

func TestLiveMutationCoordinatorPauseForGuidanceCanProceed(t *testing.T) {
	ctx := context.Background()
	store := newMutationWorkflowStore(t, "wf-live-guidance")
	now := time.Date(2026, 3, 27, 9, 15, 0, 0, time.UTC)
	require.NoError(t, archaeoevents.AppendMutationEvent(ctx, store, archaeodomain.MutationEvent{
		WorkflowID:  "wf-live-guidance",
		PlanID:      "plan-1",
		PlanVersion: intPtr(1),
		Category:    archaeodomain.MutationConfidenceChange,
		SourceKind:  "tension",
		SourceRef:   "tension-2",
		Description: "contradiction requires operator choice",
		BlastRadius: archaeodomain.BlastRadius{
			Scope:           archaeodomain.BlastRadiusStep,
			AffectedStepIDs: []string{"step-1"},
			EstimatedCount:  1,
		},
		Impact:      archaeodomain.ImpactCaution,
		Disposition: archaeodomain.DispositionPauseForGuidance,
		CreatedAt:   now,
	}))
	plan := testPlan("wf-live-guidance", now)
	step := plan.Steps["step-1"]
	state := core.NewContext()
	state.Set("euclo.execution_handoff", archaeodomain.ExecutionHandoff{
		WorkflowID:  "wf-live-guidance",
		PlanID:      "plan-1",
		PlanVersion: 1,
		CreatedAt:   now.Add(-time.Minute),
	})
	coord := execution.LiveMutationCoordinator{
		Service: execution.Service{WorkflowStore: store},
		Plans:   archaeoplans.Service{Store: preflightPlanStore{}, Now: func() time.Time { return now }},
		RequestGuidance: func(context.Context, guidance.GuidanceRequest, string) guidance.GuidanceDecision {
			return guidance.GuidanceDecision{ChoiceID: "proceed", DecidedBy: "test"}
		},
	}

	eval, err := coord.CheckpointExecution(ctx, &core.Task{Context: map[string]any{"workflow_id": "wf-live-guidance"}}, state, plan, step)
	require.NoError(t, err)
	require.NotNil(t, eval)
	require.Equal(t, archaeodomain.DispositionPauseForGuidance, eval.Disposition)
}

func TestLiveMutationCoordinatorRecordsExplicitCheckpointKind(t *testing.T) {
	ctx := context.Background()
	store := newMutationWorkflowStore(t, "wf-live-explicit")
	now := time.Date(2026, 3, 27, 9, 20, 0, 0, time.UTC)
	require.NoError(t, archaeoevents.AppendMutationEvent(ctx, store, archaeodomain.MutationEvent{
		WorkflowID:  "wf-live-explicit",
		PlanID:      "plan-1",
		Category:    archaeodomain.MutationObservation,
		SourceKind:  "comment",
		SourceRef:   "comment-2",
		Description: "supplemental note",
		BlastRadius: archaeodomain.BlastRadius{Scope: archaeodomain.BlastRadiusPlan},
		Impact:      archaeodomain.ImpactInformational,
		Disposition: archaeodomain.DispositionContinue,
		CreatedAt:   now,
	}))
	plan := testPlan("wf-live-explicit", now)
	step := plan.Steps["step-1"]
	state := core.NewContext()
	state.Set("euclo.execution_handoff", archaeodomain.ExecutionHandoff{
		WorkflowID:  "wf-live-explicit",
		PlanID:      "plan-1",
		PlanVersion: 1,
		CreatedAt:   now.Add(-time.Minute),
	})
	coord := execution.LiveMutationCoordinator{
		Service: execution.Service{WorkflowStore: store, Now: func() time.Time { return now }},
		Plans:   archaeoplans.Service{Store: preflightPlanStore{}, Now: func() time.Time { return now }},
	}

	eval, err := coord.CheckpointExecutionAt(ctx, archaeodomain.MutationCheckpointPreDispatch, &core.Task{Context: map[string]any{"workflow_id": "wf-live-explicit"}}, state, plan, step)
	require.NoError(t, err)
	require.NotNil(t, eval)
	raw, ok := state.Get("euclo.execution_mutation_checkpoint_summary")
	require.True(t, ok)
	summary, ok := raw.(archaeodomain.MutationCheckpointSummary)
	require.True(t, ok)
	require.Equal(t, archaeodomain.MutationCheckpointPreDispatch, summary.Checkpoint)
}

func testPlan(workflowID string, now time.Time) *frameworkplan.LivingPlan {
	return &frameworkplan.LivingPlan{
		ID:         "plan-1",
		WorkflowID: workflowID,
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
}
