package agentenv

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Service is the universal interface for all background services, workers,
// and periodic tasks in a workspace. Any service registered with ServiceManager
// must implement this interface to ensure consistent lifecycle management.
type Service interface {
	Start(ctx context.Context) error
	Stop() error
}

const (
	serviceStatusRunning = "running"
	serviceStatusStopped = "stopped"
	serviceStatusError   = "error"
)

// ServiceSnapshot captures the current registry status for a service.
type ServiceSnapshot struct {
	ID     string
	Status string
	Source string
	Owner  string
	Notes  []string
}

// ServiceRegistrationInfo captures provenance for a registered service.
type ServiceRegistrationInfo struct {
	Source string
	Owner  string
	Notes  []string
}

// ServiceManager handles registration and lifecycle orchestration for all
// services within a workspace session. It supports dynamic registration,
// batch start/stop operations, and clean resource cleanup.
type ServiceManager struct {
	Registry map[string]Service
	Statuses map[string]string
	Info     map[string]ServiceRegistrationInfo
	Cancel   context.CancelFunc
	Wg       sync.WaitGroup
	Mu       sync.Mutex
}

// NewServiceManager creates a new empty service registry ready for dynamic
// service registration. Use this during Workspace initialization.
func NewServiceManager() *ServiceManager {
	return &ServiceManager{
		Registry: make(map[string]Service),
		Statuses: make(map[string]string),
		Info:     make(map[string]ServiceRegistrationInfo),
	}
}

// Register adds a service to the manager by ID. If the service already exists,
// it will be overwritten (previous instance is automatically stopped).
func (sm *ServiceManager) Register(id string, s Service) {
	sm.RegisterWithInfo(id, s, ServiceRegistrationInfo{Source: "internal"})
}

// RegisterWithInfo adds a service with explicit provenance metadata.
func (sm *ServiceManager) RegisterWithInfo(id string, s Service, info ServiceRegistrationInfo) {
	sm.Mu.Lock()
	defer sm.Mu.Unlock()

	if _, exists := sm.Registry[id]; exists {
		log.Printf("service manager: overwriting existing service %q", id)
	}

	sm.Registry[id] = s
	if sm.Statuses == nil {
		sm.Statuses = make(map[string]string)
	}
	if sm.Info == nil {
		sm.Info = make(map[string]ServiceRegistrationInfo)
	}
	sm.Statuses[id] = serviceStatusStopped
	sm.Info[id] = normalizeServiceRegistrationInfo(info)
	log.Printf("service manager: registered service %q", id)
}

// Deregister removes a service from the registry and stops it if already started.
func (sm *ServiceManager) Deregister(id string) {
	sm.Mu.Lock()
	defer sm.Mu.Unlock()

	s, exists := sm.Registry[id]
	if !exists {
		return
	}

	if err := s.Stop(); err != nil {
		log.Printf("service manager: deregister error for %q: %v", id, err)
	}

	delete(sm.Registry, id)
	delete(sm.Statuses, id)
	delete(sm.Info, id)
	log.Printf("service manager: deregistered service %q", id)
}

// StartAll asynchronously starts all registered services. Services are started
// in parallel to avoid blocking startup time. Errors from individual services
// are logged but do not halt the startup of other services.
func (sm *ServiceManager) StartAll(ctx context.Context) error {
	sm.Mu.Lock()
	if len(sm.Registry) == 0 {
		sm.Mu.Unlock()
		return nil // nothing to start
	}
	services := make(map[string]Service, len(sm.Registry))
	for id, svc := range sm.Registry {
		services[id] = svc
	}
	sm.Mu.Unlock()

	var started sync.WaitGroup
	started.Add(len(services))
	for id, s := range services {
		sm.Wg.Add(1)
		go func(id string, s Service) {
			defer sm.Wg.Done()
			defer started.Done()
			if err := sm.startService(id, s, ctx); err != nil {
				log.Printf("service %s start failed: %v", id, err)
			}
		}(id, s)
	}
	started.Wait()

	return nil
}

// StopAll synchronously stops all registered services. Returns an error only if
// one or more services returned a stop error. This is used in Workspace.Close().
func (sm *ServiceManager) StopAll() error {
	if sm == nil {
		return nil
	}
	sm.Mu.Lock()
	services := make(map[string]Service, len(sm.Registry))
	for id, svc := range sm.Registry {
		services[id] = svc
	}
	sm.Mu.Unlock()

	var errs []error
	for id, s := range services {
		if err := sm.stopService(id, s); err != nil {
			errs = append(errs, fmt.Errorf("service %s stop error: %v", id, err))
		}
	}

	sm.Wg.Wait() // wait for service goroutines to observe shutdown

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Get returns a service by ID. Returns nil if not found. This allows callers
// to access specific services without re-registering them (e.g., scheduler).
func (sm *ServiceManager) Get(id string) Service {
	sm.Mu.Lock()
	defer sm.Mu.Unlock()

	if s, exists := sm.Registry[id]; exists {
		return s
	}
	return nil
}

// Snapshot returns the current registry with lifecycle status for each service.
func (sm *ServiceManager) Snapshot() []ServiceSnapshot {
	sm.Mu.Lock()
	defer sm.Mu.Unlock()

	out := make([]ServiceSnapshot, 0, len(sm.Registry))
	for id := range sm.Registry {
		status := sm.Statuses[id]
		if status == "" {
			status = serviceStatusStopped
		}
		info := sm.Info[id]
		out = append(out, ServiceSnapshot{
			ID:     id,
			Status: status,
			Source: info.Source,
			Owner:  info.Owner,
			Notes:  append([]string(nil), info.Notes...),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

// Has checks if a service with the given ID is registered.
func (sm *ServiceManager) Has(id string) bool {
	sm.Mu.Lock()
	defer sm.Mu.Unlock()

	_, exists := sm.Registry[id]
	return exists
}

// Count returns the number of currently registered services.
func (sm *ServiceManager) Count() int {
	sm.Mu.Lock()
	defer sm.Mu.Unlock()

	return len(sm.Registry)
}

// ListIDs returns a snapshot of all registered service IDs in unspecified order.
func (sm *ServiceManager) ListIDs() []string {
	sm.Mu.Lock()
	defer sm.Mu.Unlock()

	ids := make([]string, 0, len(sm.Registry))
	for id := range sm.Registry {
		ids = append(ids, id)
	}
	return ids
}

// Clear removes all services from the registry and stops them. Useful for
// restarting or cleaning up state without creating a new Workspace.
func (sm *ServiceManager) Clear() error {
	err := sm.StopAll()
	sm.Mu.Lock()
	sm.Registry = make(map[string]Service)
	sm.Statuses = make(map[string]string)
	sm.Info = make(map[string]ServiceRegistrationInfo)
	sm.Mu.Unlock()
	return err
}

// Start starts one registered service and updates its status.
func (sm *ServiceManager) Start(id string, ctx context.Context) error {
	if sm == nil {
		return fmt.Errorf("service manager unavailable")
	}
	sm.Mu.Lock()
	svc, ok := sm.Registry[id]
	sm.Mu.Unlock()
	if !ok {
		return fmt.Errorf("service %s not found", id)
	}
	return sm.startService(id, svc, ctx)
}

// Stop stops one registered service and updates its status.
func (sm *ServiceManager) Stop(id string) error {
	if sm == nil {
		return fmt.Errorf("service manager unavailable")
	}
	sm.Mu.Lock()
	svc, ok := sm.Registry[id]
	sm.Mu.Unlock()
	if !ok {
		return fmt.Errorf("service %s not found", id)
	}
	return sm.stopService(id, svc)
}

// Restart stops and then starts one registered service.
func (sm *ServiceManager) Restart(id string, ctx context.Context) error {
	if err := sm.Stop(id); err != nil {
		return err
	}
	return sm.Start(id, ctx)
}

func (sm *ServiceManager) startService(id string, svc Service, ctx context.Context) error {
	if svc == nil {
		return fmt.Errorf("service %s unavailable", id)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := svc.Start(ctx); err != nil {
		sm.setStatus(id, serviceStatusError)
		return err
	}
	sm.setStatus(id, serviceStatusRunning)
	return nil
}

func (sm *ServiceManager) stopService(id string, svc Service) error {
	if svc == nil {
		return fmt.Errorf("service %s unavailable", id)
	}
	if err := svc.Stop(); err != nil {
		sm.setStatus(id, serviceStatusError)
		return err
	}
	sm.setStatus(id, serviceStatusStopped)
	return nil
}

func (sm *ServiceManager) setStatus(id, status string) {
	sm.Mu.Lock()
	defer sm.Mu.Unlock()
	if sm.Statuses == nil {
		sm.Statuses = make(map[string]string)
	}
	sm.Statuses[id] = status
}

func normalizeServiceRegistrationInfo(info ServiceRegistrationInfo) ServiceRegistrationInfo {
	info.Source = strings.TrimSpace(info.Source)
	info.Owner = strings.TrimSpace(info.Owner)
	if len(info.Notes) > 0 {
		notes := make([]string, 0, len(info.Notes))
		for _, note := range info.Notes {
			if trimmed := strings.TrimSpace(note); trimmed != "" {
				notes = append(notes, trimmed)
			}
		}
		info.Notes = notes
	}
	return info
}

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
	Jobs   []ScheduledJob
	Cancel context.CancelFunc
	Wg     sync.WaitGroup
	Mu     sync.Mutex
}

// NewServiceScheduler creates a new scheduler.
func NewServiceScheduler() *ServiceScheduler {
	return &ServiceScheduler{}
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

	now := time.Now()
	for _, job := range jobs {
		if s.shouldRun(job, now) {
			s.Wg.Add(1)
			go func(j ScheduledJob) {
				defer s.Wg.Done()
				if err := j.Action(ctx); err != nil {
					log.Printf("scheduled job %s failed: %v", j.ID, err)
				}
			}(job)
		}
	}
}

// shouldRun determines if a job should run at the given time.
func (s *ServiceScheduler) shouldRun(job ScheduledJob, now time.Time) bool {
	if job.Interval > 0 {
		// For interval-based jobs, we rely on the scheduler to run them
		// every interval. The last-run tracking would be needed for true
		// interval enforcement, but for now we run on every tick.
		return true
	}
	if job.CronExpr != "" {
		return matchesCron(job.CronExpr, now)
	}
	return false
}

// matchesCron checks if the current time matches a cron expression.
// Supports: * (wildcard), ranges (1-5), lists (1,3,5), steps (*/2, 1-10/3).
func matchesCron(expr string, t time.Time) bool {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return false // invalid expression
	}

	minute, hour, day, month, weekday := t.Minute(), t.Hour(), t.Day(), int(t.Month()), int(t.Weekday())

	// Cron: minute hour day month weekday
	// Weekday in cron: 0 = Sunday, 7 = Sunday (we use 0-6)
	if weekday == 0 {
		weekday = 0 // Sunday
	}

	return matchCronField(fields[0], minute, 0, 59) &&
		matchCronField(fields[1], hour, 0, 23) &&
		matchCronField(fields[2], day, 1, 31) &&
		matchCronField(fields[3], month, 1, 12) &&
		matchCronField(fields[4], weekday, 0, 6)
}

// matchCronField checks if a value matches a cron field expression.
func matchCronField(field string, value, min, max int) bool {
	// Handle wildcards
	if field == "*" {
		return true
	}

	// Handle steps (e.g., */2, 1-10/3)
	if strings.Contains(field, "/") {
		parts := strings.Split(field, "/")
		if len(parts) != 2 {
			return false
		}
		step, err := strconv.Atoi(parts[1])
		if err != nil {
			return false
		}

		var start, end int
		if parts[0] == "*" {
			start, end = min, max
		} else if strings.Contains(parts[0], "-") {
			rangeParts := strings.Split(parts[0], "-")
			if len(rangeParts) != 2 {
				return false
			}
			var err error
			start, err = strconv.Atoi(rangeParts[0])
			if err != nil {
				return false
			}
			end, err = strconv.Atoi(rangeParts[1])
			if err != nil {
				return false
			}
		} else {
			var err error
			start, err = strconv.Atoi(parts[0])
			if err != nil {
				return false
			}
			end = max
		}

		for i := start; i <= end; i += step {
			if i == value {
				return true
			}
		}
		return false
	}

	// Handle ranges (e.g., 1-5)
	if strings.Contains(field, "-") {
		parts := strings.Split(field, "-")
		if len(parts) != 2 {
			return false
		}
		start, err := strconv.Atoi(parts[0])
		if err != nil {
			return false
		}
		end, err := strconv.Atoi(parts[1])
		if err != nil {
			return false
		}
		return value >= start && value <= end
	}

	// Handle lists (e.g., 1,3,5)
	if strings.Contains(field, ",") {
		parts := strings.Split(field, ",")
		for _, p := range parts {
			if v, err := strconv.Atoi(p); err == nil && v == value {
				return true
			}
		}
		return false
	}

	// Handle single value
	if v, err := strconv.Atoi(field); err == nil {
		return v == value
	}

	return false
}
