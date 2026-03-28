package runtime

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestReconstructUnitOfWorkFromCompiledExecution(t *testing.T) {
	now := time.Unix(2100, 0).UTC()
	state := core.NewContext()
	state.Set("euclo.compiled_execution", CompiledExecution{
		WorkflowID:           "wf-1",
		RunID:                "run-1",
		ExecutionID:          "exec-1",
		UnitOfWorkID:         "uow-1",
		CompiledAt:           now,
		UpdatedAt:            now,
		ModeID:               "planning",
		ObjectiveKind:        "plan_execution",
		BehaviorFamily:       "gap_analysis",
		ContextStrategyID:    "narrow_to_wide",
		VerificationPolicyID: "planning/plan_stage_execute",
		DeferralPolicyID:     "continue_with_artifacted_deferrals",
		CheckpointPolicyID:   "phase_boundary",
		PlanBinding: &UnitOfWorkPlanBinding{
			WorkflowID:    "wf-1",
			PlanID:        "plan-1",
			PlanVersion:   2,
			ActiveStepID:  "step-1",
			StepIDs:       []string{"step-1", "step-2"},
			IsPlanBacked:  true,
			IsLongRunning: true,
		},
		ContextBundle: UnitOfWorkContextBundle{
			ContextBudgetClass: "heavy",
			CompactionEligible: true,
			RestoreRequired:    true,
		},
		RoutineBindings:  []UnitOfWorkRoutineBinding{{RoutineID: "gap_analysis", Family: "gap_analysis", Required: true}},
		DeferredIssueIDs: []string{"defer-1"},
		Status:           ExecutionStatusCompletedWithDeferrals,
		ResultClass:      ExecutionResultClassCompletedWithDeferrals,
	})
	state.Set("euclo.context_compaction", ContextLifecycleState{
		WorkflowID:      "wf-1",
		RunID:           "run-1",
		Stage:           ContextLifecycleStageRestoring,
		RestoreRequired: true,
		RestoreCount:    1,
	})

	uow, ok := ReconstructUnitOfWorkFromCompiledExecution(state)
	if !ok {
		t.Fatal("expected unit of work reconstruction")
	}
	if uow.ID != "uow-1" || uow.WorkflowID != "wf-1" || uow.RunID != "run-1" {
		t.Fatalf("unexpected identity: %#v", uow)
	}
	if uow.Status != UnitOfWorkStatusRestoring {
		t.Fatalf("expected restoring status, got %q", uow.Status)
	}
	if uow.PlanBinding == nil || uow.PlanBinding.PlanID != "plan-1" || uow.PlanBinding.PlanVersion != 2 {
		t.Fatalf("unexpected plan binding: %#v", uow.PlanBinding)
	}
	if !uow.ContextBundle.RestoreRequired {
		t.Fatalf("expected restore-required context bundle: %#v", uow.ContextBundle)
	}
}

func TestBuildContextLifecycleStateRestored(t *testing.T) {
	now := time.Unix(2200, 0).UTC()
	prior := ContextLifecycleState{
		WorkflowID:         "wf-1",
		RunID:              "run-1",
		Stage:              ContextLifecycleStageRestoring,
		RestoreRequired:    true,
		RestoreCount:       1,
		CompactionEligible: true,
	}
	uow := UnitOfWork{
		ID:          "uow-1",
		WorkflowID:  "wf-1",
		RunID:       "run-1",
		ExecutionID: "exec-1",
		ContextBundle: UnitOfWorkContextBundle{
			CompactionEligible: true,
			RestoreRequired:    true,
		},
		PlanBinding: &UnitOfWorkPlanBinding{
			PlanID:      "plan-1",
			PlanVersion: 4,
		},
		DeferredIssueIDs: []string{"defer-1"},
	}

	lifecycle := BuildContextLifecycleState(uow, prior, ExecutionStatusCompleted, []string{"euclo.compiled_execution"}, now)
	if lifecycle.Stage != ContextLifecycleStageRestored {
		t.Fatalf("expected restored stage, got %q", lifecycle.Stage)
	}
	if lifecycle.ActivePlanID != "plan-1" || lifecycle.ActivePlanVersion != 4 {
		t.Fatalf("unexpected plan identity: %#v", lifecycle)
	}
	if lifecycle.LastRestoreStatus != "completed" {
		t.Fatalf("unexpected last restore status: %#v", lifecycle)
	}
	if len(lifecycle.PreservedArtifacts) != 1 || lifecycle.PreservedArtifacts[0] != "euclo.compiled_execution" {
		t.Fatalf("unexpected preserved artifact kinds: %#v", lifecycle.PreservedArtifacts)
	}
}
