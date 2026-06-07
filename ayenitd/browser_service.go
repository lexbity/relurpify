package ayenitd

import (
	"fmt"
	"strings"

	browsersvc "codeburg.org/lexbit/relurpify/ayenitd/service/browser"
	"codeburg.org/lexbit/relurpify/capability"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	fsandbox "codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/execution/agentenv"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

func registerBrowserWorkspaceService(cfg WorkspaceConfig, registration *fauthorization.AgentRegistration, registry *capability.Registry, sm *agentenv.ServiceManager, tel telemetry.Telemetry) error {
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
		sm.RegisterWithInfo("browser", browserService, agentenv.ServiceRegistrationInfo{
			Source: "ayenitd/browser_service.go",
			Owner:  "workspace",
			Notes:  []string{"registered by ayenitd", "browser.enabled=true"},
		})
	}
	return nil
}

func browserWorkspaceAgentSpec(registration *fauthorization.AgentRegistration) *agentspec.AgentRuntimeSpec {
	if registration == nil || registration.Manifest == nil {
		return nil
	}
	return registration.Manifest.Spec.Agent
}

func shouldEnableBrowserWorkspaceService(spec *agentspec.AgentRuntimeSpec) bool {
	return spec != nil && spec.Browser != nil && spec.Browser.Enabled
}

func browserDefaultBackend(spec *agentspec.AgentRuntimeSpec) string {
	if spec != nil && spec.Browser != nil {
		backend := strings.TrimSpace(spec.Browser.DefaultBackend)
		if backend != "" {
			return backend
		}
	}
	return "cdp"
}

func browserAllowedBackends(spec *agentspec.AgentRuntimeSpec) []string {
	if spec == nil || spec.Browser == nil || len(spec.Browser.AllowedBackends) == 0 {
		return nil
	}
	return append([]string(nil), spec.Browser.AllowedBackends...)
}
