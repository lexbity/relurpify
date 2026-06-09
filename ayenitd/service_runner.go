package ayenitd

import (
	"context"
	"errors"
	"fmt"

	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/execution/session"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
)

// RegisterWorkspaceServices registers workspace-owned services with the shared
// workspace service manager via the session.
func RegisterWorkspaceServices(_ context.Context, cfg WorkspaceConfig, sess *session.WorkspaceSession, capRegistry *registry.CapabilityRegistry, registration *fauthorization.AgentRegistration) error {
	if sess == nil {
		return fmt.Errorf("workspace session unavailable")
	}
	if err := registerBrowserWorkspaceService(WorkspaceConfig{Workspace: cfg.Workspace}, registration, capRegistry, &sessionServiceManager{sess}, nil); err != nil {
		return err
	}
	return nil
}

// sessionServiceManager adapts *session.WorkspaceSession to session.ServiceManager.
type sessionServiceManager struct {
	sess *session.WorkspaceSession
}

func (a *sessionServiceManager) RegisterService(id string, svc session.Service) {
	a.sess.RegisterService(id, svc)
}

func (a *sessionServiceManager) StartAll(ctx context.Context) error {
	return a.sess.StartServices(ctx)
}

func (a *sessionServiceManager) Snapshots() []session.ServiceSnapshot {
	return a.sess.ServiceSnapshots()
}

// StartWorkspaceServices starts all registered workspace services through the
// session's service manager.
func StartWorkspaceServices(ctx context.Context, sess *session.WorkspaceSession) error {
	if sess == nil {
		return fmt.Errorf("workspace session unavailable")
	}
	if err := sess.StartServices(ctx); err != nil {
		return err
	}
	var errs []error
	for _, snapshot := range sess.ServiceSnapshots() {
		if snapshot.Status == "error" {
			errs = append(errs, fmt.Errorf("service %s failed to start", snapshot.ID))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
