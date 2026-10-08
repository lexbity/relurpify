package session

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestServiceSchedulerIntervalSkipsBeforeElapsed(t *testing.T) {
	sched := NewServiceScheduler()
	var count int32
	sched.Register(ScheduledJob{
		ID:       "interval-job",
		Interval: time.Hour,
		Action: func(context.Context) error {
			atomic.AddInt32(&count, 1)
			return nil
		},
	})

	ctx := context.Background()
	sched.runJobs(ctx)
	sched.Wg.Wait()
	if got := atomic.LoadInt32(&count); got != 1 {
		t.Fatalf("first run count = %d, want 1", got)
	}

	// A second evaluation immediately after must not re-run the job: the
	// interval has not elapsed.
	sched.runJobs(ctx)
	sched.Wg.Wait()
	if got := atomic.LoadInt32(&count); got != 1 {
		t.Fatalf("second run count = %d, want 1 (interval not elapsed)", got)
	}

	sched.Mu.Lock()
	lastRun := sched.Jobs[0].LastRun
	sched.Mu.Unlock()
	if lastRun.IsZero() {
		t.Fatal("LastRun was not recorded after the job ran")
	}
}

func TestServiceSchedulerIntervalRunsAfterElapsed(t *testing.T) {
	sched := NewServiceScheduler()
	var count int32
	sched.Register(ScheduledJob{
		ID:       "interval-elapsed",
		Interval: time.Hour,
		Action: func(context.Context) error {
			atomic.AddInt32(&count, 1)
			return nil
		},
	})

	ctx := context.Background()
	sched.runJobs(ctx)
	sched.Wg.Wait()

	// Rewind LastRun to simulate an elapsed interval.
	sched.Mu.Lock()
	sched.Jobs[0].LastRun = time.Now().Add(-2 * time.Hour)
	sched.Mu.Unlock()

	sched.runJobs(ctx)
	sched.Wg.Wait()
	if got := atomic.LoadInt32(&count); got != 2 {
		t.Fatalf("count = %d, want 2 after interval elapsed", got)
	}
}

func TestServiceSchedulerNeverRunJobRunsImmediately(t *testing.T) {
	sched := NewServiceScheduler()
	ran := make(chan struct{})
	sched.Register(ScheduledJob{
		ID:       "never-run",
		Interval: 24 * time.Hour,
		Action: func(context.Context) error {
			close(ran)
			return nil
		},
	})

	sched.runJobs(context.Background())
	select {
	case <-ran:
	case <-time.After(time.Second):
		t.Fatal("job with zero LastRun did not run on first evaluation")
	}
	sched.Wg.Wait()
}
