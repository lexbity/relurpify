package euclo

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
)

func TestRefreshRuntimeExecutionArtifacts_RepairExhaustedForcesFailedExecutionStatus(t *testing.T) {
	agent := &Agent{}
	state := core.NewContext()
	state.Set("euclo.assurance_class", eucloruntime.AssuranceClassRepairExhausted)
	work := eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{ExecutionID: "exec-1",
		ModeID:               "code",
		BehaviorFamily:       "failed_verification_repair",
		VerificationPolicyID: "code/edit_verify_repair"}, ID: "uow-1",

		Status: eucloruntime.UnitOfWorkStatusExecuting,
	}

	agent.refreshRuntimeExecutionArtifacts(context.Background(), &core.Task{ID: "task-1"}, state, work, eucloruntime.ExecutionStatusCompleted, nil)

	rawStatus, ok := state.Get("euclo.execution_status")
	if !ok || rawStatus == nil {
		t.Fatal("expected execution status in state")
	}
	status, ok := rawStatus.(eucloruntime.RuntimeExecutionStatus)
	if !ok {
		t.Fatalf("expected typed execution status, got %#v", rawStatus)
	}
	if status.ResultClass != eucloruntime.ExecutionResultClassFailed {
		t.Fatalf("expected failed result class, got %q", status.ResultClass)
	}
	if status.Status != eucloruntime.ExecutionStatusFailed {
		t.Fatalf("expected failed execution status, got %q", status.Status)
	}
	if status.AssuranceClass != eucloruntime.AssuranceClassRepairExhausted {
		t.Fatalf("expected repair_exhausted assurance, got %q", status.AssuranceClass)
	}

	rawCompiled, ok := state.Get("euclo.compiled_execution")
	if !ok || rawCompiled == nil {
		t.Fatal("expected compiled execution in state")
	}
	compiled, ok := rawCompiled.(eucloruntime.CompiledExecution)
	if !ok {
		t.Fatalf("expected typed compiled execution, got %#v", rawCompiled)
	}
	if compiled.ResultClass != eucloruntime.ExecutionResultClassFailed {
		t.Fatalf("expected compiled failed result class, got %q", compiled.ResultClass)
	}
	if compiled.AssuranceClass != eucloruntime.AssuranceClassRepairExhausted {
		t.Fatalf("expected compiled repair_exhausted assurance, got %q", compiled.AssuranceClass)
	}
}

func TestRefreshRuntimeExecutionArtifacts_OperatorDeferredForcesCompletedWithDeferrals(t *testing.T) {
	agent := &Agent{}
	state := core.NewContext()
	state.Set("euclo.assurance_class", eucloruntime.AssuranceClassOperatorDeferred)
	work := eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{ExecutionID: "exec-1",
		ModeID:               "code",
		BehaviorFamily:       "failed_verification_repair",
		VerificationPolicyID: "code/edit_verify_repair"}, ID: "uow-1",

		Status: eucloruntime.UnitOfWorkStatusExecuting,
	}

	agent.refreshRuntimeExecutionArtifacts(context.Background(), &core.Task{ID: "task-1"}, state, work, eucloruntime.ExecutionStatusCompleted, nil)

	rawStatus, ok := state.Get("euclo.execution_status")
	if !ok || rawStatus == nil {
		t.Fatal("expected execution status in state")
	}
	status, ok := rawStatus.(eucloruntime.RuntimeExecutionStatus)
	if !ok {
		t.Fatalf("expected typed execution status, got %#v", rawStatus)
	}
	if status.ResultClass != eucloruntime.ExecutionResultClassCompletedWithDeferrals {
		t.Fatalf("expected completed_with_deferrals result class, got %q", status.ResultClass)
	}
	if status.Status != eucloruntime.ExecutionStatusCompletedWithDeferrals {
		t.Fatalf("expected completed_with_deferrals status, got %q", status.Status)
	}
	if status.AssuranceClass != eucloruntime.AssuranceClassOperatorDeferred {
		t.Fatalf("expected operator_deferred assurance, got %q", status.AssuranceClass)
	}
}

func TestRefreshRuntimeExecutionArtifacts_ReviewBlockedForcesBlockedExecutionStatus(t *testing.T) {
	agent := &Agent{}
	state := core.NewContext()
	state.Set("euclo.assurance_class", eucloruntime.AssuranceClassReviewBlocked)
	work := eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{ExecutionID: "exec-1",
		ModeID:               "review",
		BehaviorFamily:       "approval_assessment",
		VerificationPolicyID: "review/review_suggest_implement"}, ID: "uow-1",

		Status: eucloruntime.UnitOfWorkStatusExecuting,
	}

	agent.refreshRuntimeExecutionArtifacts(context.Background(), &core.Task{ID: "task-1"}, state, work, eucloruntime.ExecutionStatusCompleted, nil)

	rawStatus, ok := state.Get("euclo.execution_status")
	if !ok || rawStatus == nil {
		t.Fatal("expected execution status in state")
	}
	status, ok := rawStatus.(eucloruntime.RuntimeExecutionStatus)
	if !ok {
		t.Fatalf("expected typed execution status, got %#v", rawStatus)
	}
	if status.ResultClass != eucloruntime.ExecutionResultClassBlocked {
		t.Fatalf("expected blocked result class, got %q", status.ResultClass)
	}
	if status.Status != eucloruntime.ExecutionStatusBlocked {
		t.Fatalf("expected blocked status, got %q", status.Status)
	}
}

func TestRefreshRuntimeExecutionArtifacts_TDDIncompleteForcesFailedExecutionStatus(t *testing.T) {
	agent := &Agent{}
	state := core.NewContext()
	state.Set("euclo.assurance_class", eucloruntime.AssuranceClassTDDIncomplete)
	work := eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{ExecutionID: "exec-1",
		ModeID:               "tdd",
		BehaviorFamily:       "tdd_red_green_refactor",
		VerificationPolicyID: "tdd/test_driven_generation"}, ID: "uow-1",

		Status: eucloruntime.UnitOfWorkStatusExecuting,
	}

	agent.refreshRuntimeExecutionArtifacts(context.Background(), &core.Task{ID: "task-1"}, state, work, eucloruntime.ExecutionStatusCompleted, nil)

	rawStatus, ok := state.Get("euclo.execution_status")
	if !ok || rawStatus == nil {
		t.Fatal("expected execution status in state")
	}
	status, ok := rawStatus.(eucloruntime.RuntimeExecutionStatus)
	if !ok {
		t.Fatalf("expected typed execution status, got %#v", rawStatus)
	}
	if status.ResultClass != eucloruntime.ExecutionResultClassFailed {
		t.Fatalf("expected failed result class, got %q", status.ResultClass)
	}
	if status.Status != eucloruntime.ExecutionStatusFailed {
		t.Fatalf("expected failed status, got %q", status.Status)
	}
}

func TestRefreshRuntimeExecutionArtifacts_NoAssuranceClassKeepsCompletedStatus(t *testing.T) {
	agent := &Agent{}
	state := core.NewContext()
	work := eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{ExecutionID: "exec-plain",
		ModeID: "code",

		ResultClass:    eucloruntime.ExecutionResultClassCompleted,
		AssuranceClass: ""}, ID: "uow-plain",

		Status: eucloruntime.UnitOfWorkStatusExecuting,
	}

	agent.refreshRuntimeExecutionArtifacts(context.Background(), &core.Task{ID: "task-plain"}, state, work, eucloruntime.ExecutionStatusCompleted, nil)

	rawStatus, ok := state.Get("euclo.execution_status")
	if !ok || rawStatus == nil {
		t.Fatal("expected execution status in state")
	}
	status, ok := rawStatus.(eucloruntime.RuntimeExecutionStatus)
	if !ok {
		t.Fatalf("expected typed execution status, got %#v", rawStatus)
	}
	if status.Status != eucloruntime.ExecutionStatusCompleted {
		t.Fatalf("expected completed status without assurance override, got %q", status.Status)
	}
	if status.ResultClass != eucloruntime.ExecutionResultClassCompleted {
		t.Fatalf("expected completed result class, got %q", status.ResultClass)
	}
}
