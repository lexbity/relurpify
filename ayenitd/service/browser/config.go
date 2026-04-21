package browser

import (
	"context"

	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	platformbrowser "codeburg.org/lexbit/relurpify/platform/browser"
)

// BrowserServiceConfig wires the workspace-owned browser service.
type BrowserServiceConfig struct {
	WorkspaceRoot     string
	FileScope         *sandbox.FileScopePolicy
	Registration      *fauthorization.AgentRegistration
	Registry          *capability.Registry
	PermissionManager *fauthorization.PermissionManager
	AgentSpec         *core.AgentRuntimeSpec
	CommandPolicy     sandbox.CommandPolicy
	DefaultBackend    string
	AllowedBackends   []string
	Telemetry         core.Telemetry
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
