package browser

import (
	"context"

	"codeburg.org/lexbit/relurpify/capability"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
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
	Registry          *capability.CapabilityRegistry
	PermissionManager *fauthorization.PermissionManager
	AgentSpec         *agentspec.AgentRuntimeSpec
	CommandPolicy     sandbox.CommandPolicy
	DefaultBackend    string
	AllowedBackends   []string
	Telemetry         telemetry.Telemetry
	SessionFactory    func(context.Context, browserSessionConfig) (*platformbrowser.Session, error)
}

type browserSessionConfig struct {
	backendName  string
	manager      *fauthorization.PermissionManager
	agentID      string
	maxTokens    int
	registration *fauthorization.AgentRegistration
	service      *BrowserService
	paths        browserSessionPaths
}
