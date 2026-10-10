package session

import (
	"context"
	"sync"

	"codeburg.org/lexbit/relurpify/execution/services"
	"codeburg.org/lexbit/relurpify/execution/workspace"
)

// OpenMode declares feature layers to include when opening a workspace session.
type OpenMode int

const (
	// OpenModeDefault opens all configured feature layers (full scope).
	OpenModeDefault OpenMode = iota
	// OpenModeEmbeddedAgent opens only security and capabilities, no LLM
	// backend, knowledge, services, or telemetry sink.
	OpenModeEmbeddedAgent
)

// WorkspaceService is the app-facing contract for opening workspace sessions.
type WorkspaceService interface {
	OpenWorkspace(ctx context.Context, req OpenWorkspaceRequest) (*WorkspaceSession, error)
}

// OpenWorkspaceRequest captures session-level parameters for opening a workspace.
type OpenWorkspaceRequest struct {
	WorkspaceRoot string
	ConfigPath    string
	AgentName     string
	Mode          OpenMode
}

// WorkspaceSession is a coherent, open workspace session with focused
// controller access. Close is idempotent and safe to call multiple times.
type WorkspaceSession struct {
	ID        string
	Workspace workspace.Identity

	Security  SecurityController
	Knowledge KnowledgeController
	Agents    NamedAgentController
	Tools     CapabilityController
	Telemetry TelemetryView

	serviceManager sessionServiceManager
	closeOnce      sync.Once
	closeFn        func(context.Context) error
}

// sessionServiceManager is the interface for the session's internal service
// manager — the supervision substrate itself lives in execution/services.
type sessionServiceManager interface {
	RegisterService(id string, svc services.Service)
	StartAll(ctx context.Context) error
	Snapshots() []services.ServiceSnapshot
}

// SetCloseFn sets the close function for the session. Must be called before
// the first call to Close.
func (s *WorkspaceSession) SetCloseFn(fn func(context.Context) error) {
	s.closeFn = fn
}

// Close releases all resources held by the session. Idempotent and nil-safe.
func (s *WorkspaceSession) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	var err error
	s.closeOnce.Do(func() {
		if s.closeFn != nil {
			err = s.closeFn(ctx)
		}
	})
	return err
}

// SetServiceManager sets the internal service manager for the session.
func (s *WorkspaceSession) SetServiceManager(sm sessionServiceManager) {
	s.serviceManager = sm
}

// RegisterService registers a background service on the session's service manager.
func (s *WorkspaceSession) RegisterService(id string, svc services.Service) {
	if s.serviceManager != nil {
		s.serviceManager.RegisterService(id, svc)
	}
}

// StartServices starts all registered background services.
func (s *WorkspaceSession) StartServices(ctx context.Context) error {
	if s.serviceManager == nil {
		return nil
	}
	return s.serviceManager.StartAll(ctx)
}

// ServiceSnapshots returns the current status of all registered services.
func (s *WorkspaceSession) ServiceSnapshots() []services.ServiceSnapshot {
	if s.serviceManager == nil {
		return nil
	}
	return s.serviceManager.Snapshots()
}
