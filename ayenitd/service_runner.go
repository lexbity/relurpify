package ayenitd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
)

// RegisterWorkspaceServices registers workspace-owned services with the shared
// workspace service manager. The services remain owned by the workspace; this
// package only wires the lifecycle wiring required by relurpish.
func RegisterWorkspaceServices(_ context.Context, cfg WorkspaceConfig, ws *agentenv.Workspace) error {
	if ws == nil {
		return fmt.Errorf("workspace unavailable")
	}
	if ws.ServiceManager == nil {
		return fmt.Errorf("service manager unavailable")
	}
	if err := registerBrowserWorkspaceService(WorkspaceConfig{Workspace: strings.TrimSpace(cfg.Workspace)}, ws.Registration, ws.Environment.Registry, ws.ServiceManager, ws.Telemetry); err != nil {
		return err
	}
	return nil
}

// StartWorkspaceServices starts all registered workspace services through the
// workspace service manager and returns an error if any service reports a
// startup failure.
func StartWorkspaceServices(ctx context.Context, ws *agentenv.Workspace) error {
	if ws == nil {
		return fmt.Errorf("workspace unavailable")
	}
	if ws.ServiceManager == nil {
		return fmt.Errorf("service manager unavailable")
	}
	if err := ws.ServiceManager.StartAll(ctx); err != nil {
		return err
	}
	var errs []error
	for _, snapshot := range ws.ServiceManager.Snapshot() {
		if snapshot.Status == "error" {
			errs = append(errs, fmt.Errorf("service %s failed to start", snapshot.ID))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
