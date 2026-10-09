package knowledge

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// flakyStaleReporter fails its first failuresRemaining calls, then succeeds.
// It is the fault seam for the invalidation loop: a failing reporter forces
// flush to error without corrupting store state.
type flakyStaleReporter struct {
	mu                sync.Mutex
	failuresRemaining int
	reported          int
	attempts          int
}

func (r *flakyStaleReporter) ReportStaleChunks(_ context.Context, _ []ChunkID, _ []string, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attempts++
	if r.failuresRemaining > 0 {
		r.failuresRemaining--
		return errors.New("reporter unavailable")
	}
	r.reported++
	return nil
}

func (r *flakyStaleReporter) snapshot() (attempts, reported int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.attempts, r.reported
}

// waitForSubscription blocks until the pass's run loop has subscribed to the
// bus, so a single emit cannot race startup.
func waitForSubscription(t *testing.T, pass *InvalidationPass) {
	t.Helper()
	require.Eventually(t, func() bool {
		pass.stateMu.Lock()
		defer pass.stateMu.Unlock()
		return pass.eventCh != nil
	}, time.Second, time.Millisecond, "invalidation pass never subscribed")
}

// TestInvalidationPassSurvivesRepeatedFailures proves the loop reports itself
// degraded at the threshold, keeps retrying, and recovers once the fault
// clears.
func TestInvalidationPassSurvivesRepeatedFailures(t *testing.T) {
	store := newTestStore(t)
	bus := &EventBus{}
	degraded, cancel := bus.Subscribe(16)
	defer cancel()

	reporter := &flakyStaleReporter{failuresRemaining: 10}
	pass := &InvalidationPass{Store: store, Events: bus, Reporter: reporter, WorkspaceRoot: "ws"}
	require.NoError(t, pass.Start(context.Background()))
	defer func() { require.NoError(t, pass.Stop()) }()
	waitForSubscription(t, pass)

	bus.EmitChunkStaled(ChunkStaledPayload{ChunkIDs: []string{"chunk:x"}, Reason: "resilience"})

	deadline := time.Now().Add(10 * time.Second)
	for {
		_, reported := reporter.snapshot()
		if reported >= 1 {
			break
		}
		if time.Now().After(deadline) {
			attempts, _ := reporter.snapshot()
			t.Fatalf("invalidation loop never recovered (attempts=%d)", attempts)
		}
		time.Sleep(10 * time.Millisecond)
	}

	require.GreaterOrEqual(t, degradedCount(degraded), 1, "expected knowledge.invalidation_degraded at the threshold")

	// The failure counter must have reset: a fresh event is processed promptly
	// rather than waiting behind a stale backoff.
	before, _ := reporter.snapshot()
	bus.EmitChunkStaled(ChunkStaledPayload{ChunkIDs: []string{"chunk:y"}, Reason: "after-recovery"})
	require.Eventually(t, func() bool {
		after, _ := reporter.snapshot()
		return after > before
	}, time.Second, 10*time.Millisecond, "recovered loop must process new events promptly")
}

// TestInvalidationPassStopDuringBackoffIsPrompt proves Stop interrupts an
// in-flight backoff and joins the loop.
func TestInvalidationPassStopDuringBackoffIsPrompt(t *testing.T) {
	store := newTestStore(t)
	bus := &EventBus{}
	reporter := &flakyStaleReporter{failuresRemaining: 1000}
	pass := &InvalidationPass{Store: store, Events: bus, Reporter: reporter}
	require.NoError(t, pass.Start(context.Background()))
	waitForSubscription(t, pass)

	bus.EmitChunkStaled(ChunkStaledPayload{ChunkIDs: []string{"chunk:x"}, Reason: "backoff"})

	// Wait for the first failure so the loop is inside its backoff window.
	require.Eventually(t, func() bool {
		attempts, _ := reporter.snapshot()
		return attempts >= 1
	}, time.Second, 5*time.Millisecond)

	start := time.Now()
	require.NoError(t, pass.Stop())
	require.Less(t, time.Since(start), time.Second, "Stop must not wait out the backoff")
}

// TestInvalidationPassDrainFoldsQueuedEvents proves the subscriber-side drain
// moves queued bus events into the debounce buffer.
func TestInvalidationPassDrainFoldsQueuedEvents(t *testing.T) {
	store := newTestStore(t)
	pass := &InvalidationPass{Store: store, Events: &EventBus{}}

	// Wire a subscription channel directly so the drain is deterministic and
	// does not race the run loop.
	ch := make(chan Event, 4)
	pass.stateMu.Lock()
	pass.eventCh = ch
	pass.pending = make(map[ChunkID]struct{})
	pass.pendingPaths = make(map[string]struct{})
	pass.kick = make(chan struct{}, 1)
	pass.stateMu.Unlock()

	ch <- Event{Kind: EventChunkStaled, Payload: ChunkStaledPayload{ChunkIDs: []string{"chunk:drain"}, Reason: "drain"}}
	drained := pass.Drain(50 * time.Millisecond)
	require.Equal(t, 1, drained)

	ids, _, reason, ok := pass.takePending()
	require.True(t, ok)
	require.Equal(t, []ChunkID{"chunk:drain"}, ids)
	require.Equal(t, "drain", reason)

	// Draining an empty channel is a no-op.
	require.Equal(t, 0, pass.Drain(10*time.Millisecond))
}

func degradedCount(events <-chan Event) int {
	count := 0
	for {
		select {
		case event := <-events:
			if event.Kind == EventInvalidationDegraded {
				count++
			}
		default:
			return count
		}
	}
}
