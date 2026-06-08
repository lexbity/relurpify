package ayenitd

import (
	"context"
	"fmt"
	"strings"

	browsersvc "codeburg.org/lexbit/relurpify/ayenitd/service/browser"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	fsandbox "codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/execution/agentenv"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

func registerBrowserWorkspaceService(cfg WorkspaceConfig, registration *fauthorization.AgentRegistration, registry *registry.CapabilityRegistry, sm *agentenv.ServiceManager, tel telemetry.Telemetry) error {
	spec := browserWorkspaceAgentSpec(registration)
	if !shouldEnableBrowserWorkspaceService(spec) {
		return nil
	}
	if registry == nil {
		return fmt.Errorf("browser registry unavailable")
	}
	fileScope := fsandbox.NewFileScopePolicy(cfg.Workspace, nil)
	bashCfg := &fauthorization.BashConfig{}
	if spec != nil {
		bashCfg.AllowPatterns = spec.Bash.AllowPatterns
		bashCfg.DenyPatterns = spec.Bash.DenyPatterns
		bashCfg.Default = string(spec.Bash.Default)
	}
	authPolicy := fauthorization.NewCommandAuthorizationPolicy(registration.Permissions, registration.ID, bashCfg, "browser")
	cmdPolicy := fsandbox.CommandPolicyFunc(func(ctx context.Context, req fsandbox.CommandRequest) error {
		return authPolicy.CheckCommand(ctx, req.Args, req.Env)
	})
	browserService := browsersvc.New(browsersvc.BrowserServiceConfig{
		WorkspaceRoot:     cfg.Workspace,
		FileScope:         fileScope,
		Registration:      registration,
		Registry:          registry,
		PermissionManager: registration.Permissions,
		AgentSpec:         spec,
		CommandPolicy:     cmdPolicy,
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
