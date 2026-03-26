package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	fwfmp "github.com/lexcodex/relurpify/framework/middleware/fmp"
)

func TestSQLiteCircuitBreakerStoreRollsWindowAndRecoversHalfOpen(t *testing.T) {
	t.Parallel()

	store, err := NewSQLiteCircuitBreakerStore(filepath.Join(t.TempDir(), "circuit_breakers.db"))
	if err != nil {
		t.Fatalf("NewSQLiteCircuitBreakerStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.SetConfig(ctx, fwfmp.CircuitBreakerConfig{
		TrustDomain:      "mesh.remote",
		ErrorThreshold:   0.5,
		MinRequests:      2,
		WindowDuration:   10 * time.Second,
		RecoveryDuration: 5 * time.Second,
	}); err != nil {
		t.Fatalf("SetConfig() error = %v", err)
	}

	base := time.Now().UTC()
	if err := store.RecordFailure(ctx, "mesh.remote", base); err != nil {
		t.Fatalf("RecordFailure() error = %v", err)
	}
	if err := store.RecordSuccess(ctx, "mesh.remote", base.Add(11*time.Second)); err != nil {
		t.Fatalf("RecordSuccess() error = %v", err)
	}

	states, err := store.ListStates(ctx)
	if err != nil {
		t.Fatalf("ListStates() error = %v", err)
	}
	if len(states) != 1 || states[0].Requests != 1 || states[0].ErrorRate != 0 {
		t.Fatalf("states after rollover = %+v", states)
	}

	recoveryBase := time.Now().UTC()
	if err := store.RecordFailure(ctx, "mesh.remote", recoveryBase); err != nil {
		t.Fatalf("RecordFailure() second error = %v", err)
	}
	if err := store.RecordFailure(ctx, "mesh.remote", recoveryBase.Add(time.Second)); err != nil {
		t.Fatalf("RecordFailure() third error = %v", err)
	}

	// Wait until the persisted recovery window has elapsed according to wall clock, then
	// confirm GetState transitions the breaker to half-open.
	time.Sleep(6 * time.Second)

	state, err := store.GetState(ctx, "mesh.remote")
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if state != fwfmp.CircuitHalfOpen {
		t.Fatalf("state = %s, want %s", state, fwfmp.CircuitHalfOpen)
	}

	if err := store.RecordSuccess(ctx, "mesh.remote", time.Now().UTC()); err != nil {
		t.Fatalf("RecordSuccess() recovery error = %v", err)
	}
	state, err = store.GetState(ctx, "mesh.remote")
	if err != nil {
		t.Fatalf("GetState() final error = %v", err)
	}
	if state != fwfmp.CircuitClosed {
		t.Fatalf("state = %s, want %s", state, fwfmp.CircuitClosed)
	}
}
