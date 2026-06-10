package browser

import (
	"context"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	platformbrowser "codeburg.org/lexbit/relurpify/platform/browser"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// BrowserServiceConfig wires the workspace-owned browser service.
type BrowserServiceConfig struct {
	WorkspaceRoot     string
	FileScope         *permissions.FileScopePolicy
	Registration      *fauthorization.AgentRegistration
	Registry          *registry.CapabilityRegistry
	PermissionManager *fauthorization.PermissionManager
	AgentSpec         *agentspec.AgentRuntimeSpec
	CommandPolicy     sandbox.CommandPolicy
	DefaultBackend    string
	AllowedBackends   []string
	Telemetry         telemetry.Telemetry
	SessionFactory    func(context.Context, BrowserSessionConfig) (*platformbrowser.Session, error)
}

// BrowserSessionConfig configures a single browser session runtime.
type BrowserSessionConfig struct {
	BackendName  string
	Manager      *fauthorization.PermissionManager
	AgentID      string
	MaxTokens    int
	Registration *fauthorization.AgentRegistration
	LaunchRoot   string
	Policy       sandbox.CommandPolicy
}
