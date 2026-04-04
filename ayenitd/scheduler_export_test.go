package ayenitd_test

// Black-box tests for the exported ServiceScheduler API.

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/ayenitd"
)

func TestServiceScheduler_RegisterAndStop(t *testing.T) {
	s := ayenitd.NewServiceScheduler()
	s.Register(ayenitd.ScheduledJob{
		ID:       "noop",
		CronExpr: "* * * * *",
		Source:   "internal",
		Action: func(ctx context.Context) error {
			return nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	s.Stop() // must not hang
}

func TestServiceScheduler_NoJobs_StartIsNoop(t *testing.T) {
	s := ayenitd.NewServiceScheduler()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Starting with no jobs must not start background goroutines.
	// We verify by calling Stop immediately — if goroutines leaked they would
	// cause the WaitGroup to deadlock (not testable directly, but ensures no panic).
	s.Start(ctx)
	s.Stop()
}

func TestServiceScheduler_StopIsIdempotent(t *testing.T) {
	s := ayenitd.NewServiceScheduler()
	s.Stop() // stop on never-started scheduler must not panic
	s.Stop()
}

func TestServiceScheduler_StartIsIdempotent(t *testing.T) {
	var count atomic.Int64
	s := ayenitd.NewServiceScheduler()
	s.Register(ayenitd.ScheduledJob{
		ID:       "counter",
		CronExpr: "* * * * *",
		Source:   "internal",
		Action: func(ctx context.Context) error {
			count.Add(1)
			return nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	s.Start(ctx) // second Start must be a no-op, not double the goroutines
	s.Stop()
}

func TestSaveJobToMemory_RoundTrip(t *testing.T) {
	ctx := context.Background()
	mem := newTestMemory(t)

	job := ayenitd.ScheduledJob{
		ID:       "test-job",
		CronExpr: "0 2 * * *",
		Source:   "config",
	}
	if err := ayenitd.SaveJobToMemory(ctx, mem, job); err != nil {
		t.Fatalf("SaveJobToMemory: %v", err)
	}

	// Load back via scheduler
	s := ayenitd.NewServiceScheduler()
	if err := s.LoadJobsFromMemory(ctx, mem); err != nil {
		t.Fatalf("LoadJobsFromMemory: %v", err)
	}
	// We can't inspect jobs directly (unexported), but Start+Stop must not panic.
	ctx2, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	s.Start(ctx2)
	s.Stop()
}

func TestServiceScheduler_IntervalJob_FiresImmediately(t *testing.T) {
	var fired atomic.Bool
	s := ayenitd.NewServiceScheduler()
	s.Register(ayenitd.ScheduledJob{
		ID:       "instant",
		Interval: 10 * time.Second, // long enough to not fire a second time in test
		Source:   "internal",
		Action: func(ctx context.Context) error {
			fired.Store(true)
			return nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	// Give the goroutine a moment to fire the initial execution.
	deadline := time.Now().Add(500 * time.Millisecond)
	for !fired.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	s.Stop()
	if !fired.Load() {
		t.Error("interval job: expected immediate first execution on Start")
	}
}

func TestLoadJobsFromMemory_NilStore(t *testing.T) {
	s := ayenitd.NewServiceScheduler()
	// nil store must not error — memory may not be available at probe time.
	if err := s.LoadJobsFromMemory(context.Background(), nil); err != nil {
		t.Errorf("LoadJobsFromMemory(nil): expected no error, got %v", err)
	}
}
