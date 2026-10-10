package ayenitd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/execution/session"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// WorkspaceServiceDeps carries the knowledge/lifecycle dependencies the
// workspace-owned services register against. Zero values disable the
// corresponding service, so embedded scopes and unit tests stay dormant.
type WorkspaceServiceDeps struct {
	// WorkspaceRoot is the bootstrap index root. Empty falls back to the
	// WorkspaceConfig workspace path.
	WorkspaceRoot string
	// EventBus is the composition-owned knowledge bus. Nil keeps services
	// inert (they still run, but emit nowhere).
	EventBus *knowledge.EventBus
	// IndexManager drives the one-shot workspace bootstrap pass. Nil omits the
	// bootstrap service entirely.
	IndexManager *ast.IndexManager
	// Telemetry receives a boot.degraded warning when a knowledge service
	// degrades instead of aborting boot. Nil keeps the warning on the log only.
	Telemetry telemetry.Telemetry

	// IndexWorkspace and LoadStats override service internals in tests.
	IndexWorkspace func(context.Context) error
	LoadStats      func() (*ast.IndexStats, error)
}

// RegisterWorkspaceServices registers workspace-owned services with the shared
// workspace service manager via the session: the one-shot workspace bootstrap
// indexer. It starts enabled by default; it is a memory diagnostic, not
// boot-critical.
func RegisterWorkspaceServices(_ context.Context, cfg WorkspaceConfig, sess *session.WorkspaceSession, deps WorkspaceServiceDeps) error {
	if sess == nil {
		return fmt.Errorf("workspace session unavailable")
	}
	root := strings.TrimSpace(deps.WorkspaceRoot)
	if root == "" {
		root = strings.TrimSpace(cfg.Workspace)
	}
	if deps.IndexManager != nil {
		sess.RegisterService("knowledge.bootstrap", &WorkspaceBootstrapService{
			IndexManager:   deps.IndexManager,
			EventBus:       deps.EventBus,
			Telemetry:      deps.Telemetry,
			WorkspaceRoot:  root,
			IndexWorkspace: deps.IndexWorkspace,
			LoadStats:      deps.LoadStats,
		})
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
