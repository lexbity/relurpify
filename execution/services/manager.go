// Package services is the promoted supervision + scheduling substrate (Q17):
// the service manager and cron scheduler out of execution/session, so the
// runner and future consumers can supervise background services without
// dragging the workspace-session surface along. Session keeps only its glue.
package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
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

// ServiceManager provides lifecycle management for background services
// within a workspace session.
type ServiceManager interface {
	RegisterService(id string, svc Service)
	Register(id string, svc Service)
	RegisterWithInfo(id string, svc Service, info ServiceRegistrationInfo)
	Deregister(id string)
	StartAll(ctx context.Context) error
	StopAll() error
	Get(id string) Service
	Snapshot() []ServiceSnapshot
	Snapshots() []ServiceSnapshot
	Has(id string) bool
	Count() int
	ListIDs() []string
	Clear() error
	Start(ctx context.Context, id string) error
	Stop(id string) error
	Restart(ctx context.Context, id string) error
}

// serviceManager is the concrete implementation of ServiceManager.
type serviceManager struct {
	Registry map[string]Service
	Statuses map[string]string
	Info     map[string]ServiceRegistrationInfo
	Cancel   context.CancelFunc
	Wg       sync.WaitGroup
	Mu       sync.Mutex
}

// NewServiceManager creates a new empty service registry ready for dynamic
// service registration. Use this during Workspace initialization.
func NewServiceManager() ServiceManager {
	return &serviceManager{
		Registry: make(map[string]Service),
		Statuses: make(map[string]string),
		Info:     make(map[string]ServiceRegistrationInfo),
	}
}

// RegisterService registers a service and satisfies the ServiceManager interface.
func (sm *serviceManager) RegisterService(id string, s Service) {
	sm.Register(id, s)
}

// Snapshots returns the current service snapshots and satisfies the ServiceManager interface.
func (sm *serviceManager) Snapshots() []ServiceSnapshot {
	return sm.Snapshot()
}

// Register adds a service to the manager by ID. If the service already exists,
// it will be overwritten (previous instance is automatically stopped).
func (sm *serviceManager) Register(id string, s Service) {
	sm.RegisterWithInfo(id, s, ServiceRegistrationInfo{Source: "internal"})
}

// RegisterWithInfo adds a service with explicit provenance metadata.
// Overwriting an existing id stops the previous instance first — the
// registry never holds two live instances of one id.
func (sm *serviceManager) RegisterWithInfo(id string, s Service, info ServiceRegistrationInfo) {
	sm.Mu.Lock()
	previous, existed := sm.Registry[id]
	sm.Mu.Unlock()

	if existed {
		if err := previous.Stop(); err != nil {
			log.Printf("service manager: overwrite stop error for %q: %v", id, err)
		}
		log.Printf("service manager: overwriting existing service %q", id)
	}

	sm.Mu.Lock()
	defer sm.Mu.Unlock()
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
func (sm *serviceManager) Deregister(id string) {
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
func (sm *serviceManager) StartAll(ctx context.Context) error {
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
			if err := sm.startService(ctx, id, s); err != nil {
				log.Printf("service %s start failed: %v", id, err)
			}
		}(id, s)
	}
	started.Wait()

	return nil
}

// StopAll synchronously stops all registered services. Returns an error only if
// one or more services returned a stop error. This is used in Workspace.Close().
func (sm *serviceManager) StopAll() error {
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
			errs = append(errs, fmt.Errorf("service %s stop error: %w", id, err))
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
func (sm *serviceManager) Get(id string) Service {
	sm.Mu.Lock()
	defer sm.Mu.Unlock()

	if s, exists := sm.Registry[id]; exists {
		return s
	}
	return nil
}

// Snapshot returns the current registry with lifecycle status for each service.
func (sm *serviceManager) Snapshot() []ServiceSnapshot {
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
func (sm *serviceManager) Has(id string) bool {
	sm.Mu.Lock()
	defer sm.Mu.Unlock()

	_, exists := sm.Registry[id]
	return exists
}

// Count returns the number of currently registered services.
func (sm *serviceManager) Count() int {
	sm.Mu.Lock()
	defer sm.Mu.Unlock()

	return len(sm.Registry)
}

// ListIDs returns a snapshot of all registered service IDs in unspecified order.
func (sm *serviceManager) ListIDs() []string {
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
func (sm *serviceManager) Clear() error {
	err := sm.StopAll()
	sm.Mu.Lock()
	sm.Registry = make(map[string]Service)
	sm.Statuses = make(map[string]string)
	sm.Info = make(map[string]ServiceRegistrationInfo)
	sm.Mu.Unlock()
	return err
}

// Start starts one registered service and updates its status.
func (sm *serviceManager) Start(ctx context.Context, id string) error {
	if sm == nil {
		return fmt.Errorf("service manager unavailable")
	}
	sm.Mu.Lock()
	svc, ok := sm.Registry[id]
	sm.Mu.Unlock()
	if !ok {
		return fmt.Errorf("service %s not found", id)
	}
	return sm.startService(ctx, id, svc)
}

// Stop stops one registered service and updates its status.
func (sm *serviceManager) Stop(id string) error {
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
func (sm *serviceManager) Restart(ctx context.Context, id string) error {
	if err := sm.Stop(id); err != nil {
		return err
	}
	return sm.Start(ctx, id)
}

func (sm *serviceManager) startService(ctx context.Context, id string, svc Service) error {
	if svc == nil {
		return fmt.Errorf("service %s unavailable", id)
	}
	if err := svc.Start(ctx); err != nil {
		sm.setStatus(id, serviceStatusError)
		return err
	}
	sm.setStatus(id, serviceStatusRunning)
	return nil
}

func (sm *serviceManager) stopService(id string, svc Service) error {
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

func (sm *serviceManager) setStatus(id, status string) {
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
