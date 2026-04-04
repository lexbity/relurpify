package ayenitd

import (
	"context"
	"sync"
	"time"
)

// ScheduledJob represents a time-based job that the scheduler will execute.
type ScheduledJob struct {
	ID       string
	CronExpr string // standard 5-field cron expression
	Action   func(context.Context) error
	Source   string // "memory" | "config" | "internal"
}

// ServiceScheduler handles time-based and memory-triggered service invocations.
type ServiceScheduler struct {
	jobs   []ScheduledJob
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
}

// NewServiceScheduler creates a new scheduler.
func NewServiceScheduler() *ServiceScheduler {
	return &ServiceScheduler{}
}

// Register adds a job to the scheduler.
func (s *ServiceScheduler) Register(job ScheduledJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append(s.jobs, job)
}

// Start begins executing scheduled jobs.
func (s *ServiceScheduler) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.cancel != nil {
		// Already started
		return
	}
	
	// For now, we'll implement a simple ticker-based scheduler
	// In a real implementation, we would parse cron expressions
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	
	// Start a background goroutine for each job
	for _, job := range s.jobs {
		job := job // Capture for closure
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			// Simple implementation: run immediately and then every minute
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			
			// Run immediately
			if err := job.Action(ctx); err != nil {
				// Log error
			}
			
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := job.Action(ctx); err != nil {
						// Log error
					}
				}
			}
		}()
	}
}

// Stop gracefully stops the scheduler.
func (s *ServiceScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.wg.Wait()
}
