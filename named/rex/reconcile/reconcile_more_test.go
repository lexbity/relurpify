package reconcile

import (
	"context"
	"errors"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestInMemoryReconcilerGetAndDefaultResolveOutcome(t *testing.T) {
	r := &InMemoryReconciler{}
	if _, ok := r.Get("missing"); ok {
		t.Fatalf("expected missing record")
	}
	record := r.RecordAmbiguity("wf-1", "run-1", "missing ack")
	got, ok := r.Get(record.ID)
	if !ok {
		t.Fatalf("expected record to be stored")
	}
	if got.ID != record.ID {
		t.Fatalf("unexpected record: %+v", got)
	}
	resolved := r.Resolve(record, Outcome("unexpected"), "notes")
	if resolved.Status != StatusTerminal {
		t.Fatalf("unexpected terminal fallback: %+v", resolved)
	}
}

func TestValidateProtectedWriteAndAttemptRetryable(t *testing.T) {
	if err := ValidateProtectedWrite(ProtectedWrite{Resource: "x", Token: 1}, ProtectedWrite{Resource: "x", Token: 1}); err != nil {
		t.Fatalf("ValidateProtectedWrite valid: %v", err)
	}
	if err := ValidateProtectedWrite(ProtectedWrite{Resource: "x", Token: 1}, ProtectedWrite{Resource: "y", Token: 1}); err == nil {
		t.Fatalf("expected resource mismatch rejection")
	}
	if err := ValidateProtectedWrite(ProtectedWrite{}, ProtectedWrite{Resource: "x", Token: 1}); err == nil {
		t.Fatalf("expected missing resource rejection")
	}
	if !attemptRetryable(AttemptView{State: core.AttemptStateRunning}) {
		t.Fatalf("running attempt should be retryable")
	}
	for _, state := range []core.AttemptState{core.AttemptStateCompleted, core.AttemptStateFailed, core.AttemptStateOrphaned, core.AttemptStateCommittedRemote, core.AttemptStateFenced} {
		if attemptRetryable(AttemptView{State: state}) {
			t.Fatalf("state %s should not be retryable", state)
		}
	}
	if attemptRetryable(AttemptView{State: core.AttemptStateRunning, Fenced: true}) {
		t.Fatalf("fenced attempt should not be retryable")
	}
}

func TestFMPBackedReconcilerShouldRetryBranches(t *testing.T) {
	errCalled := false
	r := &FMPBackedReconciler{
		Base: &InMemoryReconciler{},
		ResolveAttempt: func(context.Context, string, string) (*AttemptView, error) {
			return nil, errors.New("boom")
		},
		ReportError: func(err error) {
			errCalled = err != nil
		},
	}
	if r.ShouldRetry(Record{LineageID: "lineage-1", AttemptID: "attempt-1"}) {
		t.Fatalf("error should suppress retry")
	}
	if !errCalled {
		t.Fatalf("expected report error")
	}

	r = &FMPBackedReconciler{
		Base: &InMemoryReconciler{},
		ResolveAttempt: func(context.Context, string, string) (*AttemptView, error) {
			return &AttemptView{State: core.AttemptStateCompleted}, nil
		},
	}
	if r.ShouldRetry(Record{LineageID: "lineage-1", AttemptID: "attempt-1"}) {
		t.Fatalf("completed attempt should not retry")
	}
	if r.ShouldRetry(Record{Status: StatusRepaired, SuppressRetry: false}) == false {
		t.Fatalf("base retry path should allow repaired records")
	}
}

func TestInMemoryProtectedWriterAndOutboxRejectInvalidInputs(t *testing.T) {
	writer := &InMemoryProtectedWriter{}
	if _, err := writer.Reserve(context.Background(), " "); err == nil {
		t.Fatalf("expected reserve to reject empty resource")
	}
	outbox := &InMemoryOutbox{}
	if err := outbox.Append(context.Background(), OutboxIntent{Key: " ", Payload: map[string]any{}}); err == nil {
		t.Fatalf("expected empty outbox key rejection")
	}
}

