package browser

import (
	"context"

	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/sandbox"
	platformbrowser "github.com/lexcodex/relurpify/platform/browser"
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
