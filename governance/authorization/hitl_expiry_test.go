package authorization

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
	"go.uber.org/goleak"
)

type recordingHITLDecisions struct {
	mu       sync.Mutex
	resolved []fwtelemetry.HITLResolution
}

func (r *recordingHITLDecisions) HITLRequested(context.Context, fwtelemetry.HITLRequest) {}

func (r *recordingHITLDecisions) PolicyEvaluated(context.Context, fwtelemetry.PolicyDecision) {}

func (r *recordingHITLDecisions) PolicyConflictShadowed(context.Context, fwtelemetry.PolicyConflict) {
}

func (r *recordingHITLDecisions) CommandEvent(context.Context, fwtelemetry.CommandEvent) {}

func (r *recordingHITLDecisions) DoomLoopDetected(context.Context, fwtelemetry.DoomLoopSignal) {}

func (r *recordingHITLDecisions) HITLResolved(_ context.Context, res fwtelemetry.HITLResolution) {
	r.mu.Lock()
	r.resolved = append(r.resolved, res)
	r.mu.Unlock()
}

func (r *recordingHITLDecisions) expiredCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, res := range r.resolved {
		if res.Outcome == "expired" {
			n++
		}
	}
	return n
}

func newExpiryTestBroker(t *testing.T) (*HITLBroker, *recordingHITLDecisions, func() time.Time) {
	t.Helper()
	decisions := &recordingHITLDecisions{}
	broker := NewHITLBroker(time.Minute, decisions)
	defer broker.Stop()
	now := time.Now()
	setClock := func() time.Time { return now }
	broker.mu.Lock()
	broker.clock = setClock
	broker.mu.Unlock()
	return broker, decisions, func() time.Time { return now }
}

// TestHITLAsyncRequestExpires: an async request that nobody answers expires
// at the broker TTL — it leaves the registry, emits an expired resolution,
// and a late approval is refused.
func TestHITLAsyncRequestExpires(t *testing.T) {
	defer goleak.VerifyNone(t)
	broker, decisions, now := newExpiryTestBroker(t)

	req := PermissionRequest{
		Permission: testPermissionDescriptor(),
		State:      "",
	}
	id, err := broker.SubmitAsync(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(broker.PendingRequests()) != 1 {
		t.Fatal("async request not registered")
	}

	// Advance past the TTL and run one sweep cycle.
	setNow := func(t time.Time) {
		broker.mu.Lock()
		broker.clock = func() time.Time { return t }
		broker.mu.Unlock()
	}
	_ = now
	later := time.Now().Add(2 * time.Minute)
	setNow(later)
	broker.expireDue()

	if len(broker.PendingRequests()) != 0 {
		t.Fatal("expired async request still pending")
	}
	if decisions.expiredCount() != 1 {
		t.Fatalf("expired resolutions = %d, want 1", decisions.expiredCount())
	}
	// Approve-after-expiry must fail: the consent window is closed.
	if err := broker.Approve(PermissionDecision{RequestID: id, Approved: true}); err == nil {
		t.Fatal("approval after expiry succeeded")
	} else if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("approve-after-expiry error must name expiry, got: %v", err)
	}
}

// TestHITLExpirySkipsBlockingWaiters: requests with a blocking waiter are
// governed by RequestPermission's own timeout, not the sweeper.
func TestHITLExpirySkipsBlockingWaiters(t *testing.T) {
	defer goleak.VerifyNone(t)
	broker, _, _ := newExpiryTestBroker(t)
	defer broker.Stop()
	events, cancel := broker.Subscribe(4)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx, cancelReq := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancelReq()
		_, _ = broker.RequestPermission(ctx, PermissionRequest{Permission: testPermissionDescriptor()})
	}()
	// Wait for the request to register, advance the clock, sweep.
	deadline := time.After(2 * time.Second)
	registered := false
	for !registered {
		select {
		case <-events:
			registered = true
		case <-deadline:
			t.Fatal("request never registered")
		default:
			if len(broker.PendingRequests()) > 0 {
				registered = true
			}
		}
	}
	broker.mu.Lock()
	broker.clock = func() time.Time { return time.Now().Add(time.Hour) }
	broker.mu.Unlock()
	broker.expireDue()
	// The blocking request must still be resolved by its own ctx timeout.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("blocking request wedged by sweeper")
	}
}

// TestHITLApproveRacesSweep: approval and expiry contend on the same request
// 100 times; exactly one side wins each round and neither panics, double-
// resolves, nor deadlocks (R9).
func TestHITLApproveRacesSweep(t *testing.T) {
	defer goleak.VerifyNone(t)
	for i := 0; i < 100; i++ {
		broker := NewHITLBroker(0, nil)
		broker.Stop()
		broker.mu.Lock()
		broker.clock = func() time.Time { return time.Unix(int64(10000+i), 0) }
		broker.mu.Unlock()

		id, err := broker.SubmitAsync(context.Background(), PermissionRequest{
			Permission: testPermissionDescriptor(),
		})
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = broker.Approve(PermissionDecision{RequestID: id, Approved: true})
		}()
		go func() {
			defer wg.Done()
			broker.mu.Lock()
			broker.clock = func() time.Time { return time.Unix(int64(10000+i), 0).Add(time.Hour) }
			broker.mu.Unlock()
			broker.expireDue()
		}()
		wg.Wait()

		// Exactly one outcome holds: either approved-and-gone or expired-and-gone.
		if len(broker.requests) != 0 {
			t.Fatalf("round %d: request leaked in registry", i)
		}
		if err := broker.Approve(PermissionDecision{RequestID: id, Approved: true}); err == nil {
			t.Fatalf("round %d: double approval accepted", i)
		}
	}
}

// TestHITLRequestPermissionDuplicateIDRejected: the blocking path enforces
// the same duplicate-ID guard as SubmitAsync.
func TestHITLRequestPermissionDuplicateIDRejected(t *testing.T) {
	defer goleak.VerifyNone(t)
	broker := NewHITLBroker(50*time.Millisecond, nil)
	defer broker.Stop()
	broker.mu.Lock()
	broker.clock = func() time.Time { return time.Unix(20000, 0) }
	broker.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = broker.RequestPermission(context.Background(), PermissionRequest{
			Permission: testPermissionDescriptor(),
		})
	}()
	// Register a request with the exact ID the next call will mint (same
	// frozen clock): the duplicate must be rejected rather than silently
	// replacing the in-flight request.
	broker.mu.Lock()
	broker.requests["hitl-20000000000000"] = &PermissionRequest{ID: "hitl-20000000000000", State: "pending"}
	broker.mu.Unlock()

	_, err := broker.RequestPermission(context.Background(), PermissionRequest{
		Permission: testPermissionDescriptor(),
	})
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected duplicate-ID rejection, got %v", err)
	}
	<-done
}

func testPermissionDescriptor() ucperms.PermissionDescriptor {
	return ucperms.PermissionDescriptor{Action: "tool:expire-test", Resource: "test"}
}
