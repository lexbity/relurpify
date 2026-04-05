package ayenitd

import (
	"context"
	"fmt"
	"io"
	"log"

	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capabilityplan"
	contractpkg "github.com/lexcodex/relurpify/framework/contract"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/policybundle"
	frameworkskills "github.com/lexcodex/relurpify/framework/skills"
)

// Workspace is a live, initialized workspace session. It holds all open
// resources. Close() must be called when the session ends. Restart() may
// be used to cleanly stop and re-start services without rebuilding stores.
type Workspace struct {
	Environment  WorkspaceEnvironment
	Registration *fauthorization.AgentRegistration

	// Internals held for Close()/Restart()
	logFile   io.Closer
	eventLog  io.Closer
	patternDB io.Closer

	// Derived fields for callers that need them
	AgentSpec            *core.AgentRuntimeSpec
	AgentDefinitions     map[string]*core.AgentDefinition
	CompiledPolicy       *policybundle.CompiledPolicyBundle
	EffectiveContract    *contractpkg.EffectiveAgentContract
	CapabilityAdmissions []capabilityplan.AdmissionResult
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
func (w *Workspace) StealClosers() (logFile, patternDB, eventLog io.Closer) {
	logFile = w.logFile
	patternDB = w.patternDB
	eventLog = w.eventLog
	w.logFile = nil
	w.patternDB = nil
	w.eventLog = nil
	return
}

// Close releases all resources held by the Workspace. This includes:
// 1. Stopping all services via ServiceManager
// 2. Closing database stores, files, and loggers
func (w *Workspace) Close() error {
	// Stop all registered services first
	if w.ServiceManager != nil {
		if err := w.ServiceManager.StopAll(); err != nil {
			return fmt.Errorf("stop services: %w", err)
		}
	}

	if w.Environment.Scheduler != nil {
		w.Environment.Scheduler.Stop()
	}

	if c, ok := w.Environment.WorkflowStore.(io.Closer); ok {
		c.Close()
	}

	if w.patternDB != nil {
		w.patternDB.Close()
	}

	if w.eventLog != nil {
		w.eventLog.Close()
	}

	if w.logFile != nil {
		w.logFile.Close()
	}

	return nil
}

// Restart cleanly stops all services and immediately re-starts them. This
// is useful for "ping" the workspace or applying configuration changes
// without dropping out of Open().
func (w *Workspace) Restart(ctx context.Context) error {
	if w.ServiceManager != nil && len(w.ServiceManager.registry) > 0 {
		log.Printf("workspace: stopping services for restart")
	}

	if err := w.Close(); err != nil {
		return err
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
	sm.mu.Lock()
	defer sm.mu.Unlock()

	result := make([]string, 0, len(sm.registry))
	for id := range sm.registry {
		result = append(result, id)
	}
	return result
}
