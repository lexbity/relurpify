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
	records, err := mem.Search(ctx, "ayenitd.cron", memory.MemoryScopeProject)
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
		
		// Phase 2 contract: action dispatch from memory records MUST go through
		// the CapabilityRegistry with full provenance tracking. Actions may NOT
		// be arbitrary Go closures loaded from persisted data — they must be
		// mapped to a registered capability ID so that PermissionManager and the
		// sandbox apply. Until Phase 2 is implemented, loaded jobs are inert.
		job.Action = func(ctx context.Context) error {
			log.Printf("scheduler: memory job %s is pending Phase 2 action dispatch implementation", job.ID)
			return nil
		}
		job.Source = "memory"
		
		s.Register(job)
		log.Printf("scheduler: loaded job %s from memory", job.ID)
	}
	
	return nil
}

// parseCronField parses a single cron field supporting:
//   - wildcard: *
//   - single value: 5
//   - range: 1-5
//   - comma list: 1,3,5 (each element may itself be a range or step)
//   - step on wildcard: */2  (every 2nd value from min to max)
//   - step on range: 1-10/3 (1, 4, 7, 10)
func parseCronField(field string, min, max int) (map[int]bool, error) {
	result := make(map[int]bool)
	if field == "" {
		return nil, fmt.Errorf("empty cron field")
	}

	for _, part := range strings.Split(field, ",") {
		if err := parseCronPart(part, min, max, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// parseCronPart handles one comma-element: plain, range, step-on-wildcard, or step-on-range.
func parseCronPart(part string, min, max int, result map[int]bool) error {
	// Step syntax: base/step  where base is * or start-end
	if idx := strings.Index(part, "/"); idx >= 0 {
		base := part[:idx]
		stepStr := part[idx+1:]
		step, err := strconv.Atoi(stepStr)
		if err != nil || step <= 0 {
			return fmt.Errorf("invalid step value in %q", part)
		}
		var start, end int
		switch {
		case base == "*":
			start, end = min, max
		case strings.Contains(base, "-"):
			var err error
			start, end, err = parseRange(base, min, max)
			if err != nil {
				return fmt.Errorf("invalid range in step %q: %w", part, err)
			}
		default:
			v, err := strconv.Atoi(base)
			if err != nil || v < min || v > max {
				return fmt.Errorf("invalid step base in %q", part)
			}
			start, end = v, max
		}
		for i := start; i <= end; i += step {
			result[i] = true
		}
		return nil
	}

	// Range syntax: start-end
	if strings.Contains(part, "-") {
		start, end, err := parseRange(part, min, max)
		if err != nil {
			return err
		}
		for i := start; i <= end; i++ {
			result[i] = true
		}
		return nil
	}

	// Wildcard
	if part == "*" {
		for i := min; i <= max; i++ {
			result[i] = true
		}
		return nil
	}

	// Single value
	val, err := strconv.Atoi(part)
	if err != nil {
		return fmt.Errorf("invalid cron value %q", part)
	}
	if val < min || val > max {
		return fmt.Errorf("cron value %d out of range [%d, %d]", val, min, max)
	}
	result[val] = true
	return nil
}

func parseRange(s string, min, max int) (start, end int, err error) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range %q", s)
	}
	start, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid range start in %q", s)
	}
	end, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid range end in %q", s)
	}
	if start < min || end > max || start > end {
		return 0, 0, fmt.Errorf("range %d-%d out of bounds [%d, %d]", start, end, min, max)
	}
	return start, end, nil
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

// Start begins executing scheduled jobs. It is a no-op if no jobs are
// registered or if the scheduler has already been started.
func (s *ServiceScheduler) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil || len(s.jobs) == 0 {
		return
	}
	
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	
	// Start a background goroutine for each job.
	for _, job := range s.jobs {
		job := job // capture for closure
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			if job.Interval > 0 {
				runIntervalJob(ctx, job)
			} else {
				runCronJob(ctx, job)
			}
		}()
	}
	
	log.Printf("scheduler: started with %d jobs", len(s.jobs))
}

// runIntervalJob runs job.Action immediately, then repeats every job.Interval.
func runIntervalJob(ctx context.Context, job ScheduledJob) {
	if err := job.Action(ctx); err != nil {
		log.Printf("scheduler: job %s failed: %v", job.ID, err)
	}
	ticker := time.NewTicker(job.Interval)
	defer ticker.Stop()
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
}

// runCronJob fires job.Action whenever the cron expression matches the current
// wall-clock minute. The cron check polls once per minute.
func runCronJob(ctx context.Context, job ScheduledJob) {
	expr := job.CronExpr
	if expr == "" {
		expr = "* * * * *"
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			matches, err := cronMatches(expr, t)
			if err != nil {
				log.Printf("scheduler: invalid cron expression for job %s: %v", job.ID, err)
				return // bad expression won't fix itself; stop this goroutine
			}
			if matches {
				if err := job.Action(ctx); err != nil {
					log.Printf("scheduler: job %s failed: %v", job.ID, err)
				}
			}
		}
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
		"interval":  job.Interval.String(),
		"source":    job.Source,
		// Note: Action cannot be serialized — see Phase 2 contract in LoadJobsFromMemory.
	}
	
	// Convert to JSON for storage
	jsonData, err := json.Marshal(value)
	if err != nil {
		return err
	}
	
	// Store in memory
	return mem.Remember(ctx, key, map[string]interface{}{
		"data": string(jsonData),
	}, memory.MemoryScopeProject)
}
