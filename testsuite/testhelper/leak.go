package testhelper

import (
	"runtime"
	"testing"
	"time"
)

// NoGoroutineLeak runs fn on the current goroutine and asserts the goroutine
// count settles back to within tolerance of the baseline shortly after fn
// returns. It is the NFR-4 guard for run-loop lifecycles: the epoch barrier and
// stream paths must not leak per-run goroutines.
func NoGoroutineLeak(t *testing.T, fn func()) {
	t.Helper()
	baseline := runtime.NumGoroutine()
	fn()
	observed, settled := waitForGoroutineSettle(baseline, time.Second)
	if !settled {
		t.Fatalf("goroutine leak: baseline %d, observed %d", baseline, observed)
	}
}

// waitForGoroutineSettle polls the goroutine count until it returns to within
// the tolerance of baseline or the timeout elapses.
func waitForGoroutineSettle(baseline int, timeout time.Duration) (int, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		observed := runtime.NumGoroutine()
		if observed <= baseline+2 {
			return observed, true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return runtime.NumGoroutine(), false
}
