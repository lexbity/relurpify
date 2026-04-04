package anitd

import (
	"context"
	"sync"
)

// ScheduledJob represents a time-based job that the scheduler will execute.
type ScheduledJob struct {
	ID       string
	CronExpr string          // standard 5-field cron expression
	Action   func(context.Context) error
	Source   string          // "memory" | "config" | "internal"
}

// ServiceScheduler handles time-based and memory-triggered service invocations.
type ServiceScheduler struct {
	jobs   []ScheduledJob
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewServiceScheduler creates a new scheduler.
func NewServiceScheduler() *ServiceScheduler {
	return &ServiceScheduler{}
}

// Register adds a job to the scheduler.
func (s *ServiceScheduler) Register(job ScheduledJob) {
	s.jobs = append(s.jobs, job)
}

// Start begins executing scheduled jobs.
func (s *ServiceScheduler) Start(ctx context.Context) {
	// TODO: Implement cron scheduling
}

// Stop gracefully stops the scheduler.
func (s *ServiceScheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}
