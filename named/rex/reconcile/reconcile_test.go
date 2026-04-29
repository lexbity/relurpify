package reconcile

import (
	"context"
	"errors"
	"testing"
)

func TestAmbiguityCreatesOperatorReviewAndSuppressesRetry(t *testing.T) {
	r := &InMemoryReconciler{}
	record := r.RecordAmbiguity("wf-1", "run-1", "missing ack")
	if record.Status != StatusOperatorReview {
		t.Fatalf("status = %q", record.Status)
	}
	if !record.SuppressRetry {
		t.Fatalf("expected retry suppression: %+v", record)
	}
	if r.ShouldRetry(record) {
		t.Fatalf("ambiguous record should not retry")
	}
}

func TestResolveRepairedClearsRetrySuppression(t *testing.T) {
	r := &InMemoryReconciler{}
	record := r.RecordAmbiguity("wf-2", "run-2", "timeout")
	record = r.Resolve(record, OutcomeRepaired, "confirmed remote completion")
	if record.Status != StatusRepaired {
		t.Fatalf("unexpected status: %+v", record)
	}
	if record.SuppressRetry {
		t.Fatalf("expected retry to be allowed: %+v", record)
	}
	if !r.ShouldRetry(record) {
		t.Fatalf("repaired record should be retryable")
	}
}

func TestResolveTerminalPreservesRetrySuppression(t *testing.T) {
	r := &InMemoryReconciler{}
	record := r.RecordAmbiguity("wf-3", "run-3", "invalid state")
	record = r.Resolve(record, OutcomeTerminal, "operator declined retry")
	if record.Status != StatusTerminal {
		t.Fatalf("unexpected status: %+v", record)
	}
	if !record.SuppressRetry {
		t.Fatalf("terminal record should suppress retry: %+v", record)
	}
}

func TestProtectedWriteRejectsStaleToken(t *testing.T) {
	err := ValidateProtectedWrite(ProtectedWrite{Resource: "x", Token: 3}, ProtectedWrite{Resource: "x", Token: 2})
	if err == nil {
		t.Fatalf("expected stale token error")
	}
}

func TestInMemoryProtectedWriterReservesIncreasingTokens(t *testing.T) {
	writer := &InMemoryProtectedWriter{}
	first, err := writer.Reserve(context.Background(), "resource-a")
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	second, err := writer.Reserve(context.Background(), "resource-a")
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if second.Token <= first.Token {
		t.Fatalf("expected increasing token values: first=%+v second=%+v", first, second)
	}
}

func TestInMemoryOutboxAppendsAtomicIntentHistory(t *testing.T) {
	outbox := &InMemoryOutbox{}
	ctx := context.Background()
	if err := outbox.Append(ctx, OutboxIntent{Key: "wf-4", WorkflowID: "wf-4", Kind: "publish", Payload: map[string]any{"step": 1}}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := outbox.Append(ctx, OutboxIntent{Key: "wf-4", WorkflowID: "wf-4", Kind: "publish", Payload: map[string]any{"step": 2}}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	intents, err := outbox.List(ctx, "wf-4")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(intents) != 2 {
		t.Fatalf("expected 2 intents, got %+v", intents)
	}
	if intents[0].Payload["step"] != 1 || intents[1].Payload["step"] != 2 {
		t.Fatalf("unexpected outbox ordering: %+v", intents)
	}
}

func TestFMPBackedReconcilerAnnotatesAmbiguityFromBinding(t *testing.T) {
	r := &FMPBackedReconciler{
		Base: &InMemoryReconciler{},
		ResolveBinding: func(context.Context, string, string) (*Binding, error) {
			return &Binding{LineageID: "lineage-1", AttemptID: "attempt-1", FencingEpoch: 7}, nil
		},
	}
	record := r.RecordAmbiguity("wf-1", "run-1", "missing ack")
	if record.LineageID != "lineage-1" || record.AttemptID != "attempt-1" || record.FencingEpoch != 7 {
		t.Fatalf("record = %+v", record)
	}
}

func TestFMPBackedReconcilerShouldRetryUsesOwnershipGroundTruth(t *testing.T) {
	r := &FMPBackedReconciler{
		Base: &InMemoryReconciler{},
		ResolveAttempt: func(context.Context, string, string) (*AttemptView, error) {
			return &AttemptView{State: AttemptStateCommittedRemote, Fenced: true}, nil
		},
	}
	record := Record{
		WorkflowID:    "wf-1",
		RunID:         "run-1",
		LineageID:     "lineage-1",
		AttemptID:     "attempt-1",
		Status:        StatusRepaired,
		SuppressRetry: false,
	}
	if r.ShouldRetry(record) {
		t.Fatalf("expected committed/fenced attempt to suppress retry")
	}
}

func TestFMPBackedReconcilerResolveAppliesOutcome(t *testing.T) {
	called := false
	r := &FMPBackedReconciler{
		Base: &InMemoryReconciler{},
		ApplyOutcome: func(_ context.Context, workflowID, runID string, outcome *Record) error {
			called = true
			if workflowID != "wf-1" || runID != "run-1" {
				t.Fatalf("unexpected workflow/run: %s %s", workflowID, runID)
			}
			if outcome.Status != StatusRepaired || outcome.LineageID != "lineage-1" {
				t.Fatalf("unexpected outcome: %+v", outcome)
			}
			return nil
		},
	}
	record := Record{
		WorkflowID:    "wf-1",
		RunID:         "run-1",
		LineageID:     "lineage-1",
		Status:        StatusOperatorReview,
		SuppressRetry: true,
	}
	resolved := r.Resolve(record, OutcomeRepaired, "confirmed")
	if !called {
		t.Fatal("expected ApplyOutcome to be called")
	}
	if resolved.Status != StatusRepaired || resolved.RepairSummary != "confirmed" {
		t.Fatalf("resolved = %+v", resolved)
	}
}

func TestFMPBackedReconcilerReportsBindingErrors(t *testing.T) {
	called := false
	r := &FMPBackedReconciler{
		Base: &InMemoryReconciler{},
		ResolveBinding: func(context.Context, string, string) (*Binding, error) {
			return nil, errors.New("boom")
		},
		ReportError: func(err error) {
			called = err != nil
		},
	}
	_ = r.RecordAmbiguity("wf-1", "run-1", "missing ack")
	if !called {
		t.Fatal("expected ReportError to be called")
	}
}
