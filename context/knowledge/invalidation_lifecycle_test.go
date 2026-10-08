package knowledge

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// recordingStaleReporter counts stale-chunk surfacing calls.
type recordingStaleReporter struct {
	mu    sync.Mutex
	calls int
}

func (r *recordingStaleReporter) ReportStaleChunks(_ context.Context, _ []ChunkID, _ []string, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return nil
}

func (r *recordingStaleReporter) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func TestInvalidationPassStopBeforeStartIsSafe(t *testing.T) {
	pass := &InvalidationPass{Events: &EventBus{}}
	require.NoError(t, pass.Stop())
}

func TestInvalidationPassStartIsNonBlocking(t *testing.T) {
	pass := &InvalidationPass{Events: &EventBus{}}
	done := make(chan error, 1)
	go func() { done <- pass.Start(context.Background()) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Start blocked; it must return once the loop is running")
	}
	require.NoError(t, pass.Stop())
}

func TestInvalidationPassStartTwiceErrors(t *testing.T) {
	pass := &InvalidationPass{Events: &EventBus{}}
	require.NoError(t, pass.Start(context.Background()))
	t.Cleanup(func() { _ = pass.Stop() })
	require.Error(t, pass.Start(context.Background()))
}

func TestInvalidationPassStopCancelsAndWaits(t *testing.T) {
	pass := &InvalidationPass{Events: &EventBus{}}
	require.NoError(t, pass.Start(context.Background()))
	require.NoError(t, pass.Stop())

	// After Stop returns, the background goroutine must have exited: the
	// WaitGroup is drained, so waiting on it returns immediately.
	drained := make(chan struct{})
	go func() {
		pass.wg.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("invalidation goroutine still running after Stop")
	}
}

func TestInvalidationPassStopHaltsProcessing(t *testing.T) {
	store := newTestStore(t)
	bus := &EventBus{}
	reporter := &recordingStaleReporter{}
	pass := &InvalidationPass{Store: store, Events: bus, Reporter: reporter}
	require.NoError(t, pass.Start(context.Background()))

	// Emit until the loop is subscribed and has surfaced a stale chunk. Each
	// iteration waits longer than the debounce window so a coalesced flush can
	// complete before the next emit resets the timer.
	deadline := time.Now().Add(2 * time.Second)
	for reporter.count() == 0 && time.Now().Before(deadline) {
		bus.EmitChunkStaled(ChunkStaledPayload{ChunkIDs: []string{"chunk:x"}, Reason: "test"})
		time.Sleep(100 * time.Millisecond)
	}
	require.Greater(t, reporter.count(), 0, "pass never surfaced a stale chunk")

	require.NoError(t, pass.Stop())
	before := reporter.count()

	bus.EmitChunkStaled(ChunkStaledPayload{ChunkIDs: []string{"chunk:y"}, Reason: "after-stop"})
	time.Sleep(150 * time.Millisecond)
	require.Equal(t, before, reporter.count())
}
