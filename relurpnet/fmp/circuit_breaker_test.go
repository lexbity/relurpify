package fmp

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryCircuitBreakerStoreRollsWindow(t *testing.T) {
	t.Parallel()

	store := &InMemoryCircuitBreakerStore{}
	if err := store.SetConfig(context.Background(), CircuitBreakerConfig{
		TrustDomain:      "mesh.remote",
		ErrorThreshold:   0.5,
		MinRequests:      2,
		WindowDuration:   10 * time.Second,
		RecoveryDuration: time.Minute,
	}); err != nil {
		t.Fatalf("SetConfig() error = %v", err)
	}

	base := time.Now().UTC()
	if err := store.RecordFailure(context.Background(), "mesh.remote", base); err != nil {
		t.Fatalf("RecordFailure() error = %v", err)
	}
	if err := store.RecordSuccess(context.Background(), "mesh.remote", base.Add(11*time.Second)); err != nil {
		t.Fatalf("RecordSuccess() error = %v", err)
	}

	states, err := store.ListStates(context.Background())
	if err != nil {
		t.Fatalf("ListStates() error = %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("states len = %d, want 1", len(states))
	}
	if states[0].Requests != 1 {
		t.Fatalf("requests = %d, want 1 after window rollover", states[0].Requests)
	}
	if states[0].ErrorRate != 0 {
		t.Fatalf("error rate = %f, want 0 after window rollover", states[0].ErrorRate)
	}
}

func TestInMemoryCircuitBreakerStoreTransitionsToHalfOpenAndRecovers(t *testing.T) {
	t.Parallel()

	store := &InMemoryCircuitBreakerStore{}
	if err := store.SetConfig(context.Background(), CircuitBreakerConfig{
		TrustDomain:      "mesh.remote",
		ErrorThreshold:   0.5,
		MinRequests:      1,
		WindowDuration:   time.Minute,
		RecoveryDuration: 5 * time.Second,
	}); err != nil {
		t.Fatalf("SetConfig() error = %v", err)
	}

	base := time.Now().UTC()
	if err := store.RecordFailure(context.Background(), "mesh.remote", base); err != nil {
		t.Fatalf("RecordFailure() error = %v", err)
	}
	state, err := store.GetState(context.Background(), "mesh.remote")
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if state != CircuitOpen {
		t.Fatalf("state = %s, want %s", state, CircuitOpen)
	}

	store.mu.Lock()
	entry := store.states["mesh.remote"]
	recovery := base.Add(-time.Second)
	entry.recoveryAt = &recovery
	store.mu.Unlock()

	state, err = store.GetState(context.Background(), "mesh.remote")
	if err != nil {
		t.Fatalf("GetState() recovery error = %v", err)
	}
	if state != CircuitHalfOpen {
		t.Fatalf("state = %s, want %s", state, CircuitHalfOpen)
	}

	if err := store.RecordSuccess(context.Background(), "mesh.remote", base.Add(time.Second)); err != nil {
		t.Fatalf("RecordSuccess() error = %v", err)
	}
	state, err = store.GetState(context.Background(), "mesh.remote")
	if err != nil {
		t.Fatalf("GetState() final error = %v", err)
	}
	if state != CircuitClosed {
		t.Fatalf("state = %s, want %s", state, CircuitClosed)
	}
}
