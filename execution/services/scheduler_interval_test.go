package services

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

// TestServiceScheduler_FakeClockSkippedTick pins the interval contract under
// clock injection: a tick that lands before the interval has elapsed does
// nothing; the tick after it does. The seam is the same `now` the production
// loop uses, so this exercises the real scheduling path.
func TestServiceScheduler_FakeClockSkippedTick(t *testing.T) {
	sched := NewServiceScheduler()
	current := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	sched.now = func() time.Time { return current }

	var count int32
	sched.Register(ScheduledJob{
		ID:       "clocked",
		Interval: 10 * time.Minute,
		Action: func(context.Context) error {
			atomic.AddInt32(&count, 1)
			return nil
		},
	})

	ctx := context.Background()
	sched.runJobs(ctx)
	sched.Wg.Wait()
	if got := atomic.LoadInt32(&count); got != 1 {
		t.Fatalf("first evaluation count = %d, want 1", got)
	}

	// +9m59s: interval not elapsed — skipped.
	current = current.Add(9*time.Minute + 59*time.Second)
	sched.runJobs(ctx)
	sched.Wg.Wait()
	if got := atomic.LoadInt32(&count); got != 1 {
		t.Fatalf("count after skipped tick = %d, want 1", got)
	}

	// +10m01s from start: due again.
	current = current.Add(2 * time.Second)
	sched.runJobs(ctx)
	sched.Wg.Wait()
	if got := atomic.LoadInt32(&count); got != 2 {
		t.Fatalf("count after elapsed interval = %d, want 2", got)
	}
}

// TestMatchesCron_SevenIsSunday pins the cron-7 Sunday alias: `* * * * 7`
// and `* * * * 0` both match a Sunday, and neither matches a Monday.
func TestMatchesCron_SevenIsSunday(t *testing.T) {
	sunday := time.Date(2026, 10, 4, 3, 30, 0, 0, time.UTC) // a Sunday
	monday := time.Date(2026, 10, 5, 3, 30, 0, 0, time.UTC)

	if !matchesCron("* * * * 7", sunday) {
		t.Fatal("cron weekday 7 must match Sunday")
	}
	if !matchesCron("* * * * 0", sunday) {
		t.Fatal("cron weekday 0 must match Sunday")
	}
	if !matchesCron("* * * * 0,7", sunday) {
		t.Fatal("cron weekday list 0,7 must match Sunday")
	}
	if matchesCron("* * * * 7", monday) {
		t.Fatal("cron weekday 7 must not match Monday")
	}
	if matchesCron("* * * * 0", monday) {
		t.Fatal("cron weekday 0 must not match Monday")
	}
	if !matchesCron("* * * * 1", monday) {
		t.Fatal("cron weekday 1 must match Monday")
	}
}
