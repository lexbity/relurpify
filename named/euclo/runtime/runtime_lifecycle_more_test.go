package runtime

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/stretchr/testify/require"
)

func TestLifecycleAndCompiledExecutionHelpers(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.context_compaction", ContextLifecycleState{WorkflowID: "wf-1", Stage: ContextLifecycleStageCompacted})
	lifecycle, ok := ContextLifecycleFromState(state)
	require.True(t, ok)
	require.Equal(t, ContextLifecycleStageCompacted, lifecycle.Stage)

	compiled := CompiledExecution{
		WorkflowID:   "wf-1",
		RunID:        "run-1",
		ExecutionID:  "exec-1",
		UnitOfWorkID: "uow-1",
		Status:       ExecutionStatusCompleted,
	}
	state.Set("euclo.compiled_execution", compiled)
	reconstructed, ok := ReconstructUnitOfWorkFromCompiledExecution(state)
	require.True(t, ok)
	require.Equal(t, "uow-1", reconstructed.ID)
	require.Equal(t, UnitOfWorkStatusCompacted, reconstructed.Status)

	task := &core.Task{Context: map[string]any{"euclo.restore_continuity": true}}
	require.True(t, RestoreRequested(task, state))

	now := time.Now().UTC()
	next := BuildContextLifecycleState(UnitOfWork{WorkflowID: "wf-1", RunID: "run-1", ExecutionID: "exec-1", ID: "uow-1", ContextBundle: UnitOfWorkContextBundle{CompactionEligible: true, RestoreRequired: true}, PlanBinding: &UnitOfWorkPlanBinding{PlanID: "plan-1", PlanVersion: 3}}, ContextLifecycleState{}, ExecutionStatusCompacted, []string{"artifact"}, now)
	require.Equal(t, ContextLifecycleStageCompacted, next.Stage)
	require.Equal(t, "plan-1", next.ActivePlanID)
	require.Equal(t, 1, next.CompactionCount)

	mark := MarkContextLifecycleRestoring(state, now)
	require.Equal(t, ContextLifecycleStageRestoring, mark.Stage)
	require.True(t, mark.RestoreRequired)

	state.Set("euclo.execution_waiver", ExecutionWaiver{WaiverID: "waiver-1", Kind: WaiverKindReviewBlock, Reason: "ok", RunID: "run-1", ArchaeoRef: "arch-1"})
	issues := BuildDeferredExecutionIssues(&guidance.DeferralPlan{Observations: []guidance.EngineeringObservation{{ID: "obs-1", Title: "Need review", Description: "follow up", BlastRadius: 2}}}, UnitOfWork{WorkflowID: "wf-1", RunID: "run-1", ExecutionID: "exec-1", PlanBinding: &UnitOfWorkPlanBinding{PlanID: "plan-1", PlanVersion: 2, ActiveStepID: "step-1"}}, state, now)
	require.NotEmpty(t, issues)
	require.Equal(t, "wf-1", issues[0].WorkflowID)
	require.Equal(t, "step-1", issues[0].StepID)
	require.NotEmpty(t, issues[0].Evidence.ShortReasoningSummary)

	SeedDeferredIssueState(state, issues)
	ids, ok := state.Get("euclo.deferred_issue_ids")
	require.True(t, ok)
	require.NotEmpty(t, ids)

	require.Equal(t, []string{"x", "y"}, uniqueStrings([]string{"x", "x", "y"}))
	require.Equal(t, "fallback", firstNonEmpty("", "fallback"))
	require.True(t, nonZeroTime(now, time.Time{}).Equal(now))
}

func TestUnitOfWorkTransitionAndHistoryHelpers(t *testing.T) {
	now := time.Now().UTC()
	previous := UnitOfWork{
		ID:                          "uow-1",
		RootID:                      "root-1",
		WorkflowID:                  "wf-1",
		RunID:                       "run-1",
		ExecutionID:                 "exec-1",
		ModeID:                      "planning",
		PrimaryRelurpicCapabilityID: "capability-a",
		PlanBinding:                 &UnitOfWorkPlanBinding{IsPlanBacked: true},
	}
	next := UnitOfWork{
		WorkflowID:                  "wf-1",
		RunID:                       "run-1",
		ExecutionID:                 "exec-1",
		ModeID:                      "planning",
		PrimaryRelurpicCapabilityID: "capability-a",
	}
	transition := ApplyUnitOfWorkTransition(previous, &next, now)
	require.True(t, transition.Preserved || transition.Rebound)
	require.NotEmpty(t, next.ID)

	rebinding := ApplyUnitOfWorkTransition(previous, &UnitOfWork{WorkflowID: "wf-1", RunID: "run-1", ExecutionID: "exec-1", ModeID: "debug", PrimaryRelurpicCapabilityID: "capability-b"}, now)
	require.NotEmpty(t, rebinding.Reason)

	history := UpdateUnitOfWorkHistory(nil, next, now)
	require.Len(t, history, 1)
	require.Equal(t, next.ID, history[0].UnitOfWorkID)

	require.True(t, shouldRebindUnitOfWork(previous, next, true, false, true))
	require.NotEmpty(t, transitionReason(previous, next, true, true, true))
	require.NotEmpty(t, transitionedUnitOfWorkID(next, now))
	preservedPrev := UnitOfWork{ID: "uow-1", ModeID: "planning", PrimaryRelurpicCapabilityID: "capability-a", TransitionState: UnitOfWorkTransitionState{PreviousUnitOfWorkID: "parent"}}
	preservedNext := UnitOfWork{ID: "uow-1", ModeID: "planning", PrimaryRelurpicCapabilityID: "capability-a"}
	require.True(t, shouldPreserveExistingTransition(preservedPrev, preservedNext))
}
