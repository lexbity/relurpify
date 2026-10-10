package services

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/telemetry"
)

// ScheduledJob represents a time-based job that the scheduler will execute.
// Exactly one of Interval or CronExpr should be set:
//   - Interval: fixed duration between executions (e.g. 6*time.Hour). Runs
//     immediately on start, then repeats. Use for period-based internal jobs.
//   - CronExpr: standard 5-field cron expression. Checked once per minute.
//     Use for time-of-day-anchored jobs (e.g. "0 2 * * *" = 02:00 daily).
//     Supports: wildcards (*), ranges (1-5), comma lists (1,3,5), steps (*/2, 1-10/3).
//
// If both are set, Interval takes precedence.
type ScheduledJob struct {
	ID       string
	Interval time.Duration // fixed-period scheduling; zero means use CronExpr
	CronExpr string        // standard 5-field cron expression
	LastRun  time.Time     // last time the job began executing; zero means never
	Action   func(context.Context) error
	Source   string // "memory" | "config" | "internal"
}

// ServiceScheduler handles time-based and memory-triggered service invocations.
type ServiceScheduler struct {
	Jobs      []ScheduledJob
	Cancel    context.CancelFunc
	Wg        sync.WaitGroup
	Mu        sync.Mutex
	telemetry telemetry.Telemetry
	running   map[string]bool // job IDs with an in-flight invocation
	// now is the clock seam (test injection); defaults to time.Now.
	now func() time.Time
}

// NewServiceScheduler creates a new scheduler.
func NewServiceScheduler() *ServiceScheduler {
	return &ServiceScheduler{running: make(map[string]bool), now: time.Now}
}

// SetTelemetry attaches the framework telemetry sink so scheduler ticks are
// observable (FR-13). A nil sink keeps the scheduler silent.
func (s *ServiceScheduler) SetTelemetry(tel telemetry.Telemetry) *ServiceScheduler {
	if s == nil {
		return nil
	}
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.telemetry = tel
	return s
}

// Register adds a job to the scheduler.
func (s *ServiceScheduler) Register(job ScheduledJob) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.Jobs = append(s.Jobs, job)
}

// Start begins the scheduler loop. It runs until the context is cancelled.
func (s *ServiceScheduler) Start(ctx context.Context) error {
	s.Mu.Lock()
	if s.Cancel != nil {
		s.Mu.Unlock()
		return fmt.Errorf("scheduler already started")
	}
	ctx, cancel := context.WithCancel(ctx)
	s.Cancel = cancel
	s.Mu.Unlock()

	s.Wg.Add(1)
	go func() {
		defer s.Wg.Done()
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		// Run immediately on start, then on ticker.
		s.runJobs(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runJobs(ctx)
			}
		}
	}()

	return nil
}

// Stop halts the scheduler.
func (s *ServiceScheduler) Stop() error {
	s.Mu.Lock()
	cancel := s.Cancel
	s.Cancel = nil
	s.Mu.Unlock()

	if cancel != nil {
		cancel()
	}
	s.Wg.Wait()
	return nil
}

// runJobs executes all jobs whose schedule has triggered.
func (s *ServiceScheduler) runJobs(ctx context.Context) {
	s.Mu.Lock()
	jobs := make([]ScheduledJob, len(s.Jobs))
	copy(jobs, s.Jobs)
	s.Mu.Unlock()

	nowFn := s.now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn()
	for _, job := range jobs {
		if !s.shouldRun(job, now) {
			continue
		}
		if !s.tryBegin(job.ID) {
			// A job still in flight when its next due time arrives is skipped
			// with a telemetry record instead of running concurrently (FR-13).
			s.emit(ctx, telemetry.EventSchedulerJobSkipped, "scheduler job skipped", map[string]any{
				"job_id": job.ID,
				"source": job.Source,
				"reason": "already_running",
			})
			continue
		}
		s.markRun(job.ID, now)
		s.emit(ctx, telemetry.EventSchedulerJobStarted, "scheduler job started", map[string]any{
			"job_id": job.ID,
			"source": job.Source,
		})
		s.Wg.Add(1)
		go func(j ScheduledJob) {
			defer s.Wg.Done()
			defer s.finish(j.ID)
			if err := j.Action(ctx); err != nil {
				s.emit(ctx, telemetry.EventSchedulerJobFailed, "scheduler job failed", map[string]any{
					"job_id": j.ID,
					"source": j.Source,
					"error":  err.Error(),
				})
				log.Printf("scheduled job %s failed: %v", j.ID, err)
				return
			}
			s.emit(ctx, telemetry.EventSchedulerJobCompleted, "scheduler job completed", map[string]any{
				"job_id": j.ID,
				"source": j.Source,
			})
		}(job)
	}
}

// tryBegin marks a job as in-flight, reporting false when it already runs.
func (s *ServiceScheduler) tryBegin(id string) bool {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if s.running[id] {
		return false
	}
	s.running[id] = true
	return true
}

// finish clears the in-flight marker for a job.
func (s *ServiceScheduler) finish(id string) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	delete(s.running, id)
}

// emit stamps correlation from ctx and dispatches one scheduler event.
func (s *ServiceScheduler) emit(ctx context.Context, eventType telemetry.EventType, message string, metadata map[string]any) {
	var sink telemetry.Telemetry
	s.Mu.Lock()
	sink = s.telemetry
	s.Mu.Unlock()
	if sink == nil {
		return
	}
	ev := telemetry.Event{
		Type:      eventType,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	}
	telemetry.StampCorrelation(ctx, &ev)
	sink.Emit(ev)
}

// shouldRun determines if a job should run at the given time.
func (s *ServiceScheduler) shouldRun(job ScheduledJob, now time.Time) bool {
	if job.Interval > 0 {
		// Strict interval enforcement: an interval job runs only once its
		// interval has elapsed since it last began. A zero LastRun (never run)
		// is always due, so jobs still run immediately on first evaluation.
		return job.LastRun.Add(job.Interval).Before(now)
	}
	if job.CronExpr != "" {
		return matchesCron(job.CronExpr, now)
	}
	return false
}

// markRun records the time a job began executing so interval scheduling can
// enforce the gap between successive runs. The scheduler loop iterates a copy
// of Jobs, so the stored slice entry must be updated by ID.
func (s *ServiceScheduler) markRun(id string, at time.Time) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	for i := range s.Jobs {
		if s.Jobs[i].ID == id {
			s.Jobs[i].LastRun = at
			return
		}
	}
}
