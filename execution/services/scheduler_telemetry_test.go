package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/telemetry"
)

// schedulerRecordingSink captures scheduler events for assertions.
type schedulerRecordingSink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *schedulerRecordingSink) Emit(event telemetry.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *schedulerRecordingSink) hasType(eventType telemetry.EventType, jobID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ev := range s.events {
		if ev.Type != eventType {
			continue
		}
		if id, _ := ev.Metadata["job_id"].(string); id != jobID {
			continue
		}
		return true
	}
	return false
}

func waitForEvent(t *testing.T, sink *schedulerRecordingSink, eventType telemetry.EventType, jobID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sink.hasType(eventType, jobID) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("event %s for job %s never observed; types=%v", eventType, jobID, sinkTypes(sink))
}

func sinkTypes(sink *schedulerRecordingSink) []telemetry.EventType {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	var out []telemetry.EventType
	for _, ev := range sink.events {
		out = append(out, ev.Type)
	}
	return out
}

func TestServiceSchedulerEmitsStartAndComplete(t *testing.T) {
	sink := &schedulerRecordingSink{}
	sched := NewServiceScheduler().SetTelemetry(sink)
	sched.Register(ScheduledJob{
		ID:       "job-complete",
		Interval: time.Hour,
		Source:   "test",
		Action: func(context.Context) error {
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.runJobs(ctx)
	sched.Wg.Wait()

	waitForEvent(t, sink, telemetry.EventSchedulerJobStarted, "job-complete")
	waitForEvent(t, sink, telemetry.EventSchedulerJobCompleted, "job-complete")
}

func TestServiceSchedulerEmitsFailure(t *testing.T) {
	sink := &schedulerRecordingSink{}
	sched := NewServiceScheduler().SetTelemetry(sink)
	sched.Register(ScheduledJob{
		ID:       "job-fail",
		Interval: time.Hour,
		Source:   "test",
		Action: func(context.Context) error {
			return errors.New("boom")
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.runJobs(ctx)
	sched.Wg.Wait()

	waitForEvent(t, sink, telemetry.EventSchedulerJobStarted, "job-fail")
	waitForEvent(t, sink, telemetry.EventSchedulerJobFailed, "job-fail")
}

// TestServiceSchedulerEmitsSkippedWhenRunning verifies that a job still in
// flight when the next tick arrives is skipped with a telemetry record instead
// of being run concurrently (FR-13, the interval-fires-every-tick bug is now
// observable).
func TestServiceSchedulerEmitsSkippedWhenRunning(t *testing.T) {
	sink := &schedulerRecordingSink{}
	sched := NewServiceScheduler().SetTelemetry(sink)

	started := make(chan struct{})
	release := make(chan struct{})
	sched.Register(ScheduledJob{
		ID:       "job-skip",
		Interval: time.Hour,
		Source:   "test",
		Action: func(context.Context) error {
			close(started)
			<-release
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sched.runJobs(ctx)
	<-started // first invocation is now in flight
	// The job's interval must appear elapsed for the in-flight job to be
	// re-evaluated; otherwise strict interval tracking skips it silently.
	sched.Mu.Lock()
	sched.Jobs[0].LastRun = time.Now().Add(-2 * time.Hour)
	sched.Mu.Unlock()
	sched.runJobs(ctx)

	waitForEvent(t, sink, telemetry.EventSchedulerJobStarted, "job-skip")
	waitForEvent(t, sink, telemetry.EventSchedulerJobSkipped, "job-skip")

	close(release)
	sched.Wg.Wait()
}

func TestServiceSchedulerWithoutTelemetryIsSilent(t *testing.T) {
	sched := NewServiceScheduler()
	sched.Register(ScheduledJob{
		ID:       "job-silent",
		Interval: time.Hour,
		Action: func(context.Context) error {
			return nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.runJobs(ctx)
	sched.Wg.Wait()
	if sched.telemetry != nil {
		t.Fatal("expected nil telemetry on a fresh scheduler")
	}
}
