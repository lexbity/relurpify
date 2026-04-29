package ayenitd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	frameworkskills "codeburg.org/lexbit/relurpify/framework/skills"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

// Workspace is a live, initialized workspace session. It holds all open
// resources. Close() must be called when the session ends. Restart() may
// be used to cleanly stop and re-start services without rebuilding stores.
type Workspace struct {
	Environment       WorkspaceEnvironment
	Registration      *fauthorization.AgentRegistration
	Backend           llm.ManagedBackend
	ProfileResolution llm.ProfileResolution

	// Internals held for Close()/Restart()
	logFile  io.Closer
	eventLog io.Closer

	// Derived fields for callers that need them
	AgentSpec            *core.AgentRuntimeSpec
	AgentDefinitions     map[string]*agentspec.AgentDefinition
	CompiledPolicy       *manifest.CompiledPolicyBundle
	EffectiveContract    *manifest.EffectiveAgentContract
	CapabilityAdmissions []capability.AdmissionResult
	SkillResults         []frameworkskills.SkillResolution

	// Observability
	Telemetry core.Telemetry
	Logger    *log.Logger

	// Service management (new for dynamic lifecycle)
	ServiceManager *ServiceManager
}

// StealClosers transfers ownership of the raw io.Closer handles from this
// Workspace to the caller. After calling, Close() will no longer close those
// resources — the caller is responsible. This is used by app/relurpish/runtime
// so that Runtime.Close() manages the lifecycle directly without double-close.
func (w *Workspace) StealClosers() (logFile, eventLog io.Closer) {
	logFile = w.logFile
	eventLog = w.eventLog
	w.logFile = nil
	w.eventLog = nil
	return
}

// Close releases all resources held by the Workspace. This includes:
// 1. Stopping all services via ServiceManager (clearing registry)
// 2. Closing database stores, files, and loggers
func (w *Workspace) Close() error {
	var errs []error

	// Stop all registered services first, but keep closing owned resources even
	// if service shutdown fails.
	if w.ServiceManager != nil {
		if err := w.ServiceManager.Clear(); err != nil {
			errs = append(errs, fmt.Errorf("stop services: %w", err))
		}
	}

	if w.Environment.Scheduler != nil {
		w.Environment.Scheduler.Stop()
	}

	if w.Backend != nil {
		if err := w.Backend.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close backend: %w", err))
		}
	}

	if w.eventLog != nil {
		if err := w.eventLog.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close event log: %w", err))
		}
	}

	if w.logFile != nil {
		if err := w.logFile.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close log file: %w", err))
		}
	}

	return errors.Join(errs...)
}

// Restart cleanly stops all services and immediately re-starts them. This
// is useful for "ping" the workspace or applying configuration changes
// without dropping out of Open().
func (w *Workspace) Restart(ctx context.Context) error {
	log.Printf("workspace: stopping services for restart")
	if err := w.stopServices(); err != nil {
		return fmt.Errorf("stop services for restart: %w", err)
	}
	if w.ServiceManager == nil {
		return fmt.Errorf("service manager unavailable")
	}
	log.Printf("workspace: restarting services")
	return w.ServiceManager.StartAll(ctx)
}

// GetService returns a specific service by ID if registered. Returns nil if
// not found. Useful for accessing the Scheduler or custom workers.
func (w *Workspace) GetService(id string) Service {
	if w.ServiceManager == nil {
		return nil
	}
	return w.ServiceManager.Get(id)
}

// ListServices returns a copy of all registered service IDs. Safe for concurrent
// calls since internal state is locked.
func (w *Workspace) ListServices() []string {
	if w.ServiceManager == nil {
		return nil
	}
	sm := w.ServiceManager
	sm.Mu.Lock()
	defer sm.Mu.Unlock()

	result := make([]string, 0, len(sm.Registry))
	for id := range sm.Registry {
		result = append(result, id)
	}
	return result
}

// stopServices stops all running services but does not clear the registry or close stores.
func (w *Workspace) stopServices() error {
	// Stop all registered services first
	if w.ServiceManager != nil {
		if err := w.ServiceManager.StopAll(); err != nil {
			return fmt.Errorf("stop services: %w", err)
		}
	}

	if w.Environment.Scheduler != nil {
		w.Environment.Scheduler.Stop()
	}
	return nil
}
