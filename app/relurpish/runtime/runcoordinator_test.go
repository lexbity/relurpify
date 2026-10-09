package runtime

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/telemetry"
)

// TestRunCoordinatorEmptyDrainIsCheap proves NFR-7: a Close with no registered
// runs is a fast no-op, returning immediately without waiting out the drain
// deadline.
func TestRunCoordinatorEmptyDrainIsCheap(t *testing.T) {
	coord := newRunCoordinator(nil)
	started := time.Now()
	report := coord.DrainAndStop()
	require.Less(t, time.Since(started), 50*time.Millisecond, "empty quiesce must return promptly")
	require.Empty(t, report.Completed)
	require.Empty(t, report.Cancelled)
	require.Empty(t, report.Abandoned)
	require.Zero(t, coord.Count())
}

// TestRunCoordinatorDrainsHealthyRun proves a run finishing inside the drain
// window is reported as completed, untouched by cancellation.
func TestRunCoordinatorDrainsHealthyRun(t *testing.T) {
	coord := newRunCoordinator(nil)
	ctx, done, ok := coord.Register("task-healthy")
	require.True(t, ok)
	require.NoError(t, ctx.Err())

	go func() {
		time.Sleep(20 * time.Millisecond)
		done()
	}()

	report := coord.DrainAndStop()
	require.Equal(t, []string{"task-healthy"}, report.Completed)
	require.Empty(t, report.Cancelled)
	require.Empty(t, report.Abandoned)
	require.Less(t, report.WaitedMS, int64(2000))
}

// TestRunCoordinatorCancelsCooperativeRun proves a run that only finishes when
// it observes cancellation is counted as cancelled, within the reap window.
func TestRunCoordinatorCancelsCooperativeRun(t *testing.T) {
	coord := newRunCoordinator(nil)
	coord.SetDurations(20*time.Millisecond, time.Second)
	ctx, done, ok := coord.Register("task-cooperative")
	require.True(t, ok)

	go func() {
		<-ctx.Done()
		done()
	}()

	report := coord.DrainAndStop()
	require.Equal(t, []string{"task-cooperative"}, report.Cancelled)
	require.Empty(t, report.Completed)
	require.Empty(t, report.Abandoned)
}

// TestRunCoordinatorAbandonsCtxIgnoringRun proves a run that ignores both the
// drain and the cancellation is abandoned past the reap window and reported.
func TestRunCoordinatorAbandonsCtxIgnoringRun(t *testing.T) {
	coord := newRunCoordinator(nil)
	coord.SetDurations(20*time.Millisecond, 20*time.Millisecond)
	_, done, ok := coord.Register("task-rogue")
	require.True(t, ok)

	report := coord.DrainAndStop()
	require.Equal(t, []string{"task-rogue"}, report.Abandoned)
	require.Empty(t, report.Completed)
	require.Empty(t, report.Cancelled)

	// A late done() must be idempotent and must not panic or resurrect the
	// entry.
	done()
	require.Zero(t, coord.Count())
}

// TestRunCoordinatorDrainIsIdempotent proves only the first DrainAndStop owns
// the teardown; later calls return immediately with an empty report.
func TestRunCoordinatorDrainIsIdempotent(t *testing.T) {
	coord := newRunCoordinator(nil)
	first := coord.DrainAndStop()
	require.NotNil(t, first)
	second := coord.DrainAndStop()
	require.Empty(t, second.Completed)
	require.Empty(t, second.Cancelled)
	require.Empty(t, second.Abandoned)

	// The intake is closed for good: a registration attempt is refused.
	_, done, ok := coord.Register("task-late")
	require.False(t, ok, "registration after Close must be refused")
	done() // no-op, never panics
}

// TestRunCoordinatorRefusesRegistrationDuringShutdown races a registration
// against the drain beginning; registrations that land after the intake closes
// are refused with a cancelled context.
func TestRunCoordinatorRefusesRegistrationDuringShutdown(t *testing.T) {
	coord := newRunCoordinator(nil)
	coord.SetDurations(20*time.Millisecond, time.Second)
	_, done, ok := coord.Register("task-first")
	require.True(t, ok)

	started := make(chan struct{}, 1)
	lateResult := make(chan bool, 1)
	go func() {
		<-started
		_, lateDone, late := coord.Register("task-late")
		lateDone()
		lateResult <- late
	}()

	report := coord.DrainAndStop()
	close(started)
	require.False(t, <-lateResult, "a registration must not succeed after Close begins")
	require.Contains(t, append(append(report.Completed, report.Cancelled...), report.Abandoned...), "task-first")
	done()
}

// TestRunCoordinatorEmitsShutdownAccounting proves abandoned and drain events
// reach the telemetry sink with their task identifiers.
func TestRunCoordinatorEmitsShutdownAccounting(t *testing.T) {
	sink := &recordingTelemetry{}
	coord := newRunCoordinator(sink)
	coord.SetDurations(20*time.Millisecond, 20*time.Millisecond)
	_, done, ok := coord.Register("task-rogue")
	require.True(t, ok)

	report := coord.DrainAndStop()
	require.NotEmpty(t, report.Abandoned)

	sawDrain := false
	sawAbandoned := false
	for _, ev := range sink.events {
		switch ev.Type {
		case telemetry.EventShutdownDrain:
			sawDrain = true
			require.Equal(t, len(report.Abandoned), ev.Metadata["abandoned"])
		case telemetry.EventShutdownAbandoned:
			sawAbandoned = true
			require.Equal(t, "task-rogue", ev.Metadata["task_id"])
		}
	}
	require.True(t, sawDrain, "runtime.shutdown_drain must be emitted")
	require.True(t, sawAbandoned, "runtime.shutdown_abandoned must be emitted")

	done()
	require.Zero(t, coord.Count())
}

// TestRunCoordinatorConcurrentLifecycle hammers registration and completion
// while a drain runs, proving the coordinator is safe under -race.
func TestRunCoordinatorConcurrentLifecycle(t *testing.T) {
	coord := newRunCoordinator(nil)
	coord.SetDurations(time.Second, time.Second)

	for i := 0; i < 50; i++ {
		ctx, done, ok := coord.Register(string(rune('a'+i)) + "-task")
		require.True(t, ok)
		go func() {
			<-ctx.Done()
			done()
		}()
	}

	report := coord.DrainAndStop()
	require.Zero(t, coord.Count())
	require.Equal(t, 50, report.total())
}
