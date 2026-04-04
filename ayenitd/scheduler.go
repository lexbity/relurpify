package ayenitd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/memory"
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

// LoadJobsFromMemory queries the memory store for persisted job definitions
// under the well-known key prefix "ayenitd.cron.*".
func (s *ServiceScheduler) LoadJobsFromMemory(ctx context.Context, mem memory.MemoryStore) error {
	if mem == nil {
		return nil
	}
	
	// Search for cron jobs in memory store
	records, err := mem.Search(ctx, "ayenitd.cron", memory.ScopeWorkspace)
	if err != nil {
		// If Search is not implemented, just log and continue
		log.Printf("scheduler: memory store search not available: %v", err)
		return nil
	}
	
	for _, record := range records {
		// Each record should have a JSON payload with job definition
		data := record.Value
		if data == nil {
			continue
		}
		
		// Try to extract job fields
		var job ScheduledJob
		if id, ok := data["id"].(string); ok {
			job.ID = id
		} else {
			continue
		}
		if cronExpr, ok := data["cron_expr"].(string); ok {
			job.CronExpr = cronExpr
		} else {
			continue
		}
		
		// Action cannot be serialized, so we need to map ID to a predefined action
		// For now, we'll register a placeholder action
		job.Action = func(ctx context.Context) error {
			log.Printf("scheduler: executing memory job %s", job.ID)
			// TODO: Implement actual action based on job type
			return nil
		}
		job.Source = "memory"
		
		s.Register(job)
		log.Printf("scheduler: loaded job %s from memory", job.ID)
	}
	
	return nil
}

// parseCronField parses a single cron field (minute, hour, day of month, month, day of week)
func parseCronField(field string, min, max int) (map[int]bool, error) {
	result := make(map[int]bool)
	
	// Handle wildcard
	if field == "*" {
		for i := min; i <= max; i++ {
			result[i] = true
		}
		return result, nil
	}
	
	// Handle comma-separated values
	parts := strings.Split(field, ",")
	for _, part := range parts {
		// Handle ranges
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("invalid range: %s", part)
			}
			start, err1 := strconv.Atoi(rangeParts[0])
			end, err2 := strconv.Atoi(rangeParts[1])
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("invalid range numbers: %s", part)
			}
			if start < min || end > max || start > end {
				return nil, fmt.Errorf("range out of bounds: %s", part)
			}
			for i := start; i <= end; i++ {
				result[i] = true
			}
		} else {
			// Single number
			val, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid number: %s", part)
			}
			if val < min || val > max {
				return nil, fmt.Errorf("value out of bounds: %d", val)
			}
			result[val] = true
		}
	}
	return result, nil
}

// cronMatches checks if the given time matches the cron expression
func cronMatches(cronExpr string, t time.Time) (bool, error) {
	fields := strings.Fields(cronExpr)
	if len(fields) != 5 {
		return false, fmt.Errorf("cron expression must have 5 fields")
	}
	
	// Parse each field
	minuteMap, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return false, fmt.Errorf("minute field: %w", err)
	}
	hourMap, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return false, fmt.Errorf("hour field: %w", err)
	}
	dayMap, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return false, fmt.Errorf("day field: %w", err)
	}
	monthMap, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return false, fmt.Errorf("month field: %w", err)
	}
	weekdayMap, err := parseCronField(fields[4], 0, 6) // 0 = Sunday, 6 = Saturday
	if err != nil {
		return false, fmt.Errorf("weekday field: %w", err)
	}
	
	// Check if current time matches
	if !minuteMap[t.Minute()] {
		return false, nil
	}
	if !hourMap[t.Hour()] {
		return false, nil
	}
	if !dayMap[t.Day()] {
		return false, nil
	}
	if !monthMap[int(t.Month())] {
		return false, nil
	}
	if !weekdayMap[int(t.Weekday())] {
		return false, nil
	}
	
	return true, nil
}

// Start begins executing scheduled jobs.
func (s *ServiceScheduler) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.cancel != nil {
		// Already started
		return
	}
	
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	
	// Start a background goroutine for each job
	for _, job := range s.jobs {
		job := job // Capture for closure
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			
			// If cron expression is empty or "* * * * *", run every minute
			if job.CronExpr == "" || job.CronExpr == "* * * * *" {
				// Simple implementation for每分钟
				ticker := time.NewTicker(time.Minute)
				defer ticker.Stop()
				
				// Run immediately
				if err := job.Action(ctx); err != nil {
					log.Printf("scheduler: job %s failed: %v", job.ID, err)
				}
				
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						if err := job.Action(ctx); err != nil {
							log.Printf("scheduler: job %s failed: %v", job.ID, err)
						}
					}
				}
			} else {
				// Parse cron expression and run at specified times
				ticker := time.NewTicker(time.Minute)
				defer ticker.Stop()
				
				for {
					select {
					case <-ctx.Done():
						return
					case t := <-ticker.C:
						matches, err := cronMatches(job.CronExpr, t)
						if err != nil {
							log.Printf("scheduler: invalid cron expression for job %s: %v", job.ID, err)
							continue
						}
						if matches {
							if err := job.Action(ctx); err != nil {
								log.Printf("scheduler: job %s failed: %v", job.ID, err)
							}
						}
					}
				}
			}
		}()
	}
	
	log.Printf("scheduler: started with %d jobs", len(s.jobs))
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
	log.Printf("scheduler: stopped")
}

// SaveJobToMemory stores a job definition in the memory store.
func SaveJobToMemory(ctx context.Context, mem memory.MemoryStore, job ScheduledJob) error {
	if mem == nil {
		return fmt.Errorf("memory store is nil")
	}
	
	key := fmt.Sprintf("ayenitd.cron.%s", job.ID)
	value := map[string]interface{}{
		"id":        job.ID,
		"cron_expr": job.CronExpr,
		"source":    job.Source,
		// Note: Action cannot be serialized
	}
	
	// Convert to JSON for storage
	jsonData, err := json.Marshal(value)
	if err != nil {
		return err
	}
	
	// Store in memory
	return mem.Remember(ctx, key, map[string]interface{}{
		"data": string(jsonData),
	}, memory.ScopeWorkspace)
}
