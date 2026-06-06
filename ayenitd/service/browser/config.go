package browser

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	platformbrowser "codeburg.org/lexbit/relurpify/platform/browser"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// BrowserServiceConfig wires the workspace-owned browser service.
type BrowserServiceConfig struct {
	WorkspaceRoot     string
	FileScope         *sandbox.FileScopePolicy
	Registration      *fauthorization.AgentRegistration
	Registry          *capability.Registry
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
