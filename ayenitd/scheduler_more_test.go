package ayenitd

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/memory"
	"github.com/stretchr/testify/require"
)

type schedulerMemoryStub struct {
	records []memory.MemoryRecord
	err     error
}

func (m schedulerMemoryStub) Remember(context.Context, string, map[string]interface{}, memory.MemoryScope) error {
	return nil
}
func (m schedulerMemoryStub) Recall(context.Context, string, memory.MemoryScope) (*memory.MemoryRecord, bool, error) {
	return nil, false, nil
}
func (m schedulerMemoryStub) Search(context.Context, string, memory.MemoryScope) ([]memory.MemoryRecord, error) {
	if m.err != nil {
		return nil, m.err
	}
	return append([]memory.MemoryRecord(nil), m.records...), nil
}
func (m schedulerMemoryStub) Forget(context.Context, string, memory.MemoryScope) error { return nil }
func (m schedulerMemoryStub) Summarize(context.Context, memory.MemoryScope) (string, error) {
	return "", nil
}

func TestLoadJobsFromMemoryHandlesSearchError(t *testing.T) {
	s := NewServiceScheduler()
	require.NoError(t, s.LoadJobsFromMemory(context.Background(), schedulerMemoryStub{err: errors.New("search boom")}))
	require.Len(t, s.jobs, 0)
}

func TestLoadJobsFromMemorySkipsMalformedRecords(t *testing.T) {
	s := NewServiceScheduler()
	mem := schedulerMemoryStub{
		records: []memory.MemoryRecord{
			{Value: map[string]interface{}{"cron_expr": "* * * * *"}},
			{Value: map[string]interface{}{"id": "bad"}},
		},
	}
	require.NoError(t, s.LoadJobsFromMemory(context.Background(), mem))
	require.Len(t, s.jobs, 0)
}

func TestLoadJobsFromMemoryRegistersValidJobs(t *testing.T) {
	s := NewServiceScheduler()
	mem := schedulerMemoryStub{
		records: []memory.MemoryRecord{
			{
				Value: map[string]interface{}{
					"id":        "job-1",
					"cron_expr": "0 2 * * *",
				},
			},
		},
	}
	require.NoError(t, s.LoadJobsFromMemory(context.Background(), mem))
	require.Len(t, s.jobs, 1)
	require.Equal(t, "job-1", s.jobs[0].ID)
	require.Equal(t, "memory", s.jobs[0].Source)
}

type fakeSchedulerTicker struct {
	c    chan time.Time
	stop atomic.Bool
}

func (t *fakeSchedulerTicker) C() <-chan time.Time { return t.c }
func (t *fakeSchedulerTicker) Stop()               { t.stop.Store(true) }

func TestSchedulerRunIntervalAndCronJobs(t *testing.T) {
	origNewTickerFn := newTickerFn
	t.Cleanup(func() { newTickerFn = origNewTickerFn })

	t.Run("interval fires and stops", func(t *testing.T) {
		tick := &fakeSchedulerTicker{c: make(chan time.Time, 1)}
		newTickerFn = func(time.Duration) schedulerTicker { return tick }

		var fired atomic.Int32
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			runIntervalJob(ctx, ScheduledJob{
				ID:       "interval",
				Interval: time.Second,
				Action: func(context.Context) error {
					fired.Add(1)
					return nil
				},
			})
			close(done)
		}()
		require.Eventually(t, func() bool { return fired.Load() == 1 }, time.Second, 10*time.Millisecond)
		tick.c <- time.Now()
		require.Eventually(t, func() bool { return fired.Load() == 2 }, time.Second, 10*time.Millisecond)
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("interval job did not stop")
		}
		require.True(t, tick.stop.Load())
	})

	t.Run("interval logs action error", func(t *testing.T) {
		tick := &fakeSchedulerTicker{c: make(chan time.Time, 1)}
		newTickerFn = func(time.Duration) schedulerTicker { return tick }

		var fired atomic.Int32
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			runIntervalJob(ctx, ScheduledJob{
				ID:       "interval-error",
				Interval: time.Second,
				Action: func(context.Context) error {
					fired.Add(1)
					if fired.Load() == 1 {
						return errors.New("boom")
					}
					return nil
				},
			})
			close(done)
		}()
		require.Eventually(t, func() bool { return fired.Load() == 1 }, time.Second, 10*time.Millisecond)
		tick.c <- time.Now()
		require.Eventually(t, func() bool { return fired.Load() == 2 }, time.Second, 10*time.Millisecond)
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("interval error job did not stop")
		}
	})

	t.Run("cron fires and invalid expression stops", func(t *testing.T) {
		tick := &fakeSchedulerTicker{c: make(chan time.Time, 1)}
		newTickerFn = func(time.Duration) schedulerTicker { return tick }

		var fired atomic.Int32
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			runCronJob(ctx, ScheduledJob{
				ID:       "cron",
				CronExpr: "* * * * *",
				Action: func(context.Context) error {
					fired.Add(1)
					return nil
				},
			})
			close(done)
		}()
		tick.c <- time.Date(2026, 4, 10, 12, 34, 0, 0, time.UTC)
		require.Eventually(t, func() bool { return fired.Load() == 1 }, time.Second, 10*time.Millisecond)
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("cron job did not stop")
		}
		require.True(t, tick.stop.Load())

		tick2 := &fakeSchedulerTicker{c: make(chan time.Time, 1)}
		newTickerFn = func(time.Duration) schedulerTicker { return tick2 }
		done = make(chan struct{})
		go func() {
			runCronJob(context.Background(), ScheduledJob{
				ID:       "cron-invalid",
				CronExpr: "bad expr",
				Action:   func(context.Context) error { return nil },
			})
			close(done)
		}()
		tick2.c <- time.Now()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("invalid cron expression did not stop")
		}

		tick3 := &fakeSchedulerTicker{c: make(chan time.Time, 1)}
		newTickerFn = func(time.Duration) schedulerTicker { return tick3 }
		done = make(chan struct{})
		ctx3, cancel3 := context.WithCancel(context.Background())
		go func() {
			runCronJob(ctx3, ScheduledJob{
				ID:       "cron-miss",
				CronExpr: "0 0 1 1 *",
				Action:   func(context.Context) error { return nil },
			})
			close(done)
		}()
		tick3.c <- time.Date(2026, 4, 10, 12, 34, 0, 0, time.UTC)
		cancel3()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("cron no-match job did not stop")
		}
	})
}

func TestSaveJobToMemoryNilStore(t *testing.T) {
	err := SaveJobToMemory(context.Background(), nil, ScheduledJob{ID: "job"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "memory store is nil")
}
