// services.go contains the Service interface, ServiceManager registry,
// and utility functions for dynamic service lifecycle management.
// This replaces the current placeholder with full implementation.
package ayenitd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
)

// Service is the universal interface for all background services, workers,
// and periodic tasks in ayenitd. Any service registered with ServiceManager
// must implement this interface to ensure consistent lifecycle management.
type Service interface {
	Start(ctx context.Context) error
	Stop() error
}

// ServiceManager handles registration and lifecycle orchestration for all
// services within a workspace session. It supports dynamic registration,
// batch start/stop operations, and clean resource cleanup.
type ServiceManager struct {
	registry map[string]Service
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.Mutex
}

// NewServiceManager creates a new empty service registry ready for dynamic
// service registration. Use this during Workspace initialization in Open().
func NewServiceManager() *ServiceManager {
	return &ServiceManager{
		registry: make(map[string]Service),
	}
}

// Register adds a service to the manager by ID. If the service already exists,
// it will be overwritten (previous instance is automatically stopped).
func (sm *ServiceManager) Register(id string, s Service) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.registry[id]; exists {
		log.Printf("service manager: overwriting existing service %q", id)
	}

	sm.registry[id] = s
	log.Printf("service manager: registered service %q", id)
}

// Deregister removes a service from the registry and stops it if already started.
func (sm *ServiceManager) Deregister(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	s, exists := sm.registry[id]
	if !exists {
		return
	}

	if err := s.Stop(); err != nil {
		log.Printf("service manager: deregister error for %q: %v", id, err)
	}

	delete(sm.registry, id)
	log.Printf("service manager: deregistered service %q", id)
}

// StartAll asynchronously starts all registered services. Services are started
// in parallel to avoid blocking startup time. Errors from individual services
// are logged but do not halt the startup of other services.
func (sm *ServiceManager) StartAll(ctx context.Context) error {
	sm.mu.Lock()
	if len(sm.registry) == 0 {
		sm.mu.Unlock()
		return nil // nothing to start
	}
	services := make(map[string]Service, len(sm.registry))
	for id, svc := range sm.registry {
		services[id] = svc
	}
	sm.mu.Unlock()

	var started sync.WaitGroup
	started.Add(len(services))
	for id, s := range services {
		sm.wg.Add(1)
		go func(id string, s Service) {
			defer sm.wg.Done()
			defer started.Done()
			if err := s.Start(ctx); err != nil {
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
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var errs []error
	for id, s := range sm.registry {
		if err := s.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("service %s stop error: %v", id, err))
		}
	}

	sm.wg.Wait() // wait for all stop operations to complete

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Get returns a service by ID. Returns nil if not found. This allows callers
// to access specific services without re-registering them (e.g., scheduler).
func (sm *ServiceManager) Get(id string) Service {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if s, exists := sm.registry[id]; exists {
		return s
	}
	return nil
}

// Has checks if a service with the given ID is registered.
func (sm *ServiceManager) Has(id string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	_, exists := sm.registry[id]
	return exists
}

// Count returns the number of currently registered services.
func (sm *ServiceManager) Count() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	return len(sm.registry)
}

// ListIDs returns a snapshot of all registered service IDs in unspecified order.
func (sm *ServiceManager) ListIDs() []string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	ids := make([]string, 0, len(sm.registry))
	for id := range sm.registry {
		ids = append(ids, id)
	}
	return ids
}

// Clear removes all services from the registry and stops them. Useful for
// restarting or cleaning up state without creating a new Workspace.
func (sm *ServiceManager) Clear() error {
	err := sm.StopAll()
	sm.mu.Lock()
	sm.registry = make(map[string]Service)
	sm.mu.Unlock()
	return err
}
