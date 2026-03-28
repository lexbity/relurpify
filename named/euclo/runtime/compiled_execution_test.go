package runtime

import (
	"testing"
	"time"
)

func TestBuildCompiledExecutionFromUnitOfWork(t *testing.T) {
	now := time.Unix(789, 0).UTC()
	uow := UnitOfWork{
		ID:                   "uow-1",
		WorkflowID:           "wf-1",
		RunID:                "run-1",
		ExecutionID:          "exec-1",
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
			ActiveStepID:  "step-2",
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
		CreatedAt:        now,
	}
	status := BuildRuntimeExecutionStatus(uow, ExecutionStatusCompletedWithDeferrals, ExecutionResultClassCompletedWithDeferrals, now)
	compiled := BuildCompiledExecution(uow, status, uow.CreatedAt)
	if compiled.WorkflowID != "wf-1" || compiled.RunID != "run-1" {
		t.Fatalf("unexpected identity: %#v", compiled)
	}
	if compiled.PlanBinding == nil || compiled.PlanBinding.PlanID != "plan-1" {
		t.Fatalf("unexpected plan binding: %#v", compiled.PlanBinding)
	}
	if compiled.ResultClass != ExecutionResultClassCompletedWithDeferrals {
		t.Fatalf("unexpected result class: %q", compiled.ResultClass)
	}
	if compiled.Status != ExecutionStatusCompletedWithDeferrals {
		t.Fatalf("unexpected status: %q", compiled.Status)
	}
}

func TestResultClassForOutcome(t *testing.T) {
	if got := ResultClassForOutcome(ExecutionStatusCompleted, nil, nil); got != ExecutionResultClassCompleted {
		t.Fatalf("got %q", got)
	}
	if got := ResultClassForOutcome(ExecutionStatusCompleted, []string{"defer-1"}, nil); got != ExecutionResultClassCompletedWithDeferrals {
		t.Fatalf("got %q", got)
	}
	if got := ResultClassForOutcome(ExecutionStatusBlocked, nil, nil); got != ExecutionResultClassBlocked {
		t.Fatalf("got %q", got)
	}
}

func TestStatusForResultClass(t *testing.T) {
	if got := StatusForResultClass(ExecutionStatusCompleted, ExecutionResultClassCompletedWithDeferrals); got != ExecutionStatusCompletedWithDeferrals {
		t.Fatalf("got %q", got)
	}
	if got := StatusForResultClass(ExecutionStatusFailed, ExecutionResultClassCanceled); got != ExecutionStatusCanceled {
		t.Fatalf("got %q", got)
	}
	if got := StatusForResultClass(ExecutionStatusCompleted, ExecutionResultClassCompleted); got != ExecutionStatusCompleted {
		t.Fatalf("got %q", got)
	}
}
