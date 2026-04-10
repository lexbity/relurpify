package ayenitd

import (
	"context"
	"fmt"
	"strings"

	browsersvc "github.com/lexcodex/relurpify/ayenitd/service/browser"
	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	fsandbox "github.com/lexcodex/relurpify/framework/sandbox"
)

func registerBrowserWorkspaceService(ctx context.Context, cfg WorkspaceConfig, registration *fauthorization.AgentRegistration, registry *capability.Registry, sm *ServiceManager, tel core.Telemetry) error {
	spec := browserWorkspaceAgentSpec(registration)
	if !shouldEnableBrowserWorkspaceService(spec) {
		return nil
	}
	if registry == nil {
		return fmt.Errorf("browser registry unavailable")
	}
	fileScope := fsandbox.NewFileScopePolicy(cfg.Workspace, nil)
	browserService := browsersvc.New(browsersvc.BrowserServiceConfig{
		WorkspaceRoot:     cfg.Workspace,
		FileScope:         fileScope,
		Registration:      registration,
		Registry:          registry,
		PermissionManager: registration.Permissions,
		AgentSpec:         spec,
		CommandPolicy:     fauthorization.NewCommandAuthorizationPolicy(registration.Permissions, registration.ID, spec, "browser"),
		DefaultBackend:    browserDefaultBackend(spec),
		AllowedBackends:   browserAllowedBackends(spec),
		Telemetry:         tel,
	})
	if sm != nil {
		sm.Register("browser", browserService)
	}
	if err := browserService.Start(ctx); err != nil {
		return fmt.Errorf("start browser service: %w", err)
	}
	return nil
}

func browserWorkspaceAgentSpec(registration *fauthorization.AgentRegistration) *core.AgentRuntimeSpec {
	if registration == nil || registration.Manifest == nil {
		return nil
	}
	return registration.Manifest.Spec.Agent
}

func shouldEnableBrowserWorkspaceService(spec *core.AgentRuntimeSpec) bool {
	return spec != nil && spec.Browser != nil && spec.Browser.Enabled
}

func browserDefaultBackend(spec *core.AgentRuntimeSpec) string {
	if spec != nil && spec.Browser != nil {
		backend := strings.TrimSpace(spec.Browser.DefaultBackend)
		if backend != "" {
			return backend
		}
	}
	return "cdp"
}

func browserAllowedBackends(spec *core.AgentRuntimeSpec) []string {
	if spec == nil || spec.Browser == nil || len(spec.Browser.AllowedBackends) == 0 {
		return nil
	}
	return append([]string(nil), spec.Browser.AllowedBackends...)
}
