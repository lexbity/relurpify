package persistence

import (
	"context"
	"testing"
)

func TestWriterPersistReturnsErrorOnRejection(t *testing.T) {
	w := NewWriter(nil, nil, nil, nil)

	result, err := w.Persist(context.Background(), PersistenceRequest{})
	if err == nil {
		t.Fatal("expected non-nil error for a rejected persistence request")
	}
	if result == nil {
		t.Fatal("expected the result to be returned alongside the rejection error")
	}
	if result.Action != ActionRejected {
		t.Fatalf("action = %q, want %q", result.Action, ActionRejected)
	}
	if result.Error == nil {
		t.Fatal("result.Error must be populated on rejection")
	}
}

func TestWriterPersistBatchReturnsErrorOnAnyRejection(t *testing.T) {
	w := NewWriter(nil, nil, nil, nil)

	results, err := w.PersistBatch(context.Background(), []PersistenceRequest{{}})
	if err == nil {
		t.Fatal("expected aggregate error when a batch item is rejected")
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Action != ActionRejected {
		t.Fatalf("results[0].Action = %q, want %q", results[0].Action, ActionRejected)
	}
}
