package testhelper

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestNoGoroutineLeakPass proves the helper returns when the goroutine count
// settles back to baseline.
func TestNoGoroutineLeakPass(t *testing.T) {
	NoGoroutineLeak(t, func() { time.Sleep(5 * time.Millisecond) })
}

// TestWaitForGoroutineSettleSettles proves the settle core returns success when
// the count returns to baseline.
func TestWaitForGoroutineSettleSettles(t *testing.T) {
	baseline := runtime.NumGoroutine()
	_, settled := waitForGoroutineSettle(baseline, 100*time.Millisecond)
	require.True(t, settled)
}

// TestWaitForGoroutineSettleDetectsLeak proves the settle core reports failure
// when a goroutine outlives the timeout.
func TestWaitForGoroutineSettleDetectsLeak(t *testing.T) {
	stop := make(chan struct{})
	defer close(stop)
	started := make(chan struct{}, 3)

	baseline := runtime.NumGoroutine()
	for i := 0; i < 3; i++ {
		go func() {
			started <- struct{}{}
			<-stop
		}()
	}
	for i := 0; i < 3; i++ {
		<-started
	}

	observed, settled := waitForGoroutineSettle(baseline, 40*time.Millisecond)
	require.False(t, settled, "goroutines outliving the window must be reported")
	require.Greater(t, observed, baseline+2)
}
