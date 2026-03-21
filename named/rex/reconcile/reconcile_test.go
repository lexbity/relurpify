package reconcile

import (
	"context"
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
