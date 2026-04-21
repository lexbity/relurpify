package work_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/runtime/work"
)

// ---------------------------------------------------------------------------
// ResultClassForOutcome
// ---------------------------------------------------------------------------

func TestResultClassForOutcome_CompletedWithNoIssues(t *testing.T) {
	rc := work.ResultClassForOutcome(work.ExecutionStatusCompleted, nil, nil)
	if rc != work.ExecutionResultClassCompleted {
		t.Fatalf("expected Completed, got %v", rc)
	}
}

func TestResultClassForOutcome_CompletedWithDeferrals(t *testing.T) {
	rc := work.ResultClassForOutcome(work.ExecutionStatusCompleted, []string{"issue-1"}, nil)
	if rc != work.ExecutionResultClassCompletedWithDeferrals {
		t.Fatalf("expected CompletedWithDeferrals, got %v", rc)
	}
}

func TestResultClassForOutcome_FailedStatus(t *testing.T) {
	rc := work.ResultClassForOutcome(work.ExecutionStatusFailed, nil, errors.New("oops"))
	if rc != work.ExecutionResultClassFailed {
		t.Fatalf("expected Failed, got %v", rc)
	}
}

func TestResultClassForOutcome_CanceledViaContextError(t *testing.T) {
	rc := work.ResultClassForOutcome(work.ExecutionStatusCanceled, nil, context.Canceled)
	if rc != work.ExecutionResultClassCanceled {
		t.Fatalf("expected Canceled, got %v", rc)
	}
}

func TestResultClassForOutcome_RestoreFailedStatus(t *testing.T) {
	rc := work.ResultClassForOutcome(work.ExecutionStatusRestoreFailed, nil, nil)
	if rc != work.ExecutionResultClassRestoreFailed {
		t.Fatalf("expected RestoreFailed, got %v", rc)
	}
}

func TestResultClassForOutcome_BlockedStatus(t *testing.T) {
	rc := work.ResultClassForOutcome(work.ExecutionStatusBlocked, nil, nil)
	if rc != work.ExecutionResultClassBlocked {
		t.Fatalf("expected Blocked, got %v", rc)
	}
}

// ---------------------------------------------------------------------------
// StatusForResultClass
// ---------------------------------------------------------------------------

func TestStatusForResultClass_CompletedResultClassPreservesStatus(t *testing.T) {
	s := work.StatusForResultClass(work.ExecutionStatusCompleted, work.ExecutionResultClassCompleted)
	if s != work.ExecutionStatusCompleted {
		t.Fatalf("got %v", s)
	}
}

func TestStatusForResultClass_CompletedWithDeferrals(t *testing.T) {
	s := work.StatusForResultClass(work.ExecutionStatusCompleted, work.ExecutionResultClassCompletedWithDeferrals)
	if s != work.ExecutionStatusCompletedWithDeferrals {
		t.Fatalf("got %v", s)
	}
}

func TestStatusForResultClass_BlockedResultClass(t *testing.T) {
	s := work.StatusForResultClass(work.ExecutionStatusExecuting, work.ExecutionResultClassBlocked)
	if s != work.ExecutionStatusBlocked {
		t.Fatalf("got %v", s)
	}
}

func TestStatusForResultClass_FailedResultClass(t *testing.T) {
	s := work.StatusForResultClass(work.ExecutionStatusExecuting, work.ExecutionResultClassFailed)
	if s != work.ExecutionStatusFailed {
		t.Fatalf("got %v", s)
	}
}

// ---------------------------------------------------------------------------
// BuildCompiledExecution / BuildRuntimeExecutionStatus — smoke tests
// (verify the forwarding functions compile and run without panic)
// ---------------------------------------------------------------------------

func TestBuildRuntimeExecutionStatus_DoesNotPanic(t *testing.T) {
	uow := work.UnitOfWork{ExecutionDescriptor: work.ExecutionDescriptor{ModeID: "chat"}, ID: "uow-1"}
	_ = work.BuildRuntimeExecutionStatus(uow, work.ExecutionStatusCompleted, work.ExecutionResultClassCompleted, time.Now())
}

func TestBuildCompiledExecution_DoesNotPanic(t *testing.T) {
	uow := work.UnitOfWork{ID: "uow-1"}
	status := work.BuildRuntimeExecutionStatus(uow, work.ExecutionStatusCompleted, work.ExecutionResultClassCompleted, time.Now())
	_ = work.BuildCompiledExecution(uow, status, time.Now())
}

// ---------------------------------------------------------------------------
// SeedCompiledExecutionState — smoke test
// ---------------------------------------------------------------------------

func TestSeedCompiledExecutionState_DoesNotPanic(t *testing.T) {
	uow := work.UnitOfWork{ID: "uow-seed"}
	status := work.BuildRuntimeExecutionStatus(uow, work.ExecutionStatusCompleted, work.ExecutionResultClassCompleted, time.Now())
	state := &stubSetter{}
	work.SeedCompiledExecutionState(state, uow, status)
}

type stubSetter struct {
	keys []string
}

func (s *stubSetter) Set(key string, _ any) {
	s.keys = append(s.keys, key)
}
