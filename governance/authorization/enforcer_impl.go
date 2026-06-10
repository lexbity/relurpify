package authorization

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
)

// permissionManagerEnforcer adapts PermissionManager to the Enforcer interface.
type permissionManagerEnforcer struct {
	pm *PermissionManager
}

// NewEnforcer creates an Enforcer from a PermissionManager.
func NewEnforcer(pm *PermissionManager) Enforcer {
	return &permissionManagerEnforcer{pm: pm}
}

// Check evaluates an AccessRequest and returns a Decision.
// It is safe for concurrent use and performs no I/O.
func (e *permissionManagerEnforcer) Check(ctx context.Context, req governanceports.AccessRequest) governanceports.Decision {
	if e.pm == nil {
		return governanceports.Decision{Allow: false, Reason: "enforcer not initialized"}
	}
	agentID := req.Principal.AgentID
	if agentID == "" {
		agentID = governanceports.PrincipalFromContext(ctx).AgentID
	}

	switch req.Action {
	case governanceports.ActionFileRead:
		return e.checkFile(ctx, agentID, "read", req.Resource.ID)
	case governanceports.ActionFileWrite:
		return e.checkFile(ctx, agentID, "write", req.Resource.ID)
	case governanceports.ActionFileExecute:
		return e.checkFile(ctx, agentID, "execute", req.Resource.ID)
	case governanceports.ActionExecBinary:
		return e.checkExecutable(ctx, agentID, req.Resource.ID)
	case governanceports.ActionNetEgress:
		return e.checkNetwork(ctx, agentID, req.Resource.ID)
	case governanceports.ActionToolInvoke:
		return e.checkTool(ctx, agentID, req.Resource.ID)
	case governanceports.ActionCapability:
		return e.checkCapability(ctx, agentID, req.Resource.ID)
	case governanceports.ActionIPC:
		return e.checkIPC(ctx, agentID, req.Resource.ID)
	default:
		return governanceports.Decision{Allow: false, Reason: fmt.Sprintf("unknown action: %s", req.Action)}
	}
}

func (e *permissionManagerEnforcer) checkFile(ctx context.Context, agentID, action, path string) governanceports.Decision {
	var act permissions.FileSystemAction
	switch action {
	case "read":
		act = permissions.FileSystemRead
	case "write":
		act = permissions.FileSystemWrite
	case "execute":
		act = permissions.FileSystemExecute
	default:
		return governanceports.Decision{Allow: false, Reason: "unsupported filesystem action: " + action}
	}
	if err := e.pm.CheckFileAccess(ctx, agentID, act, path); err != nil {
		return governanceports.Decision{Allow: false, Reason: err.Error()}
	}
	return governanceports.Decision{Allow: true, Reason: "file " + action + " allowed"}
}

func (e *permissionManagerEnforcer) checkExecutable(ctx context.Context, agentID, binary string) governanceports.Decision {
	if err := e.pm.CheckExecutable(ctx, agentID, binary, nil, nil); err != nil {
		return governanceports.Decision{Allow: false, Reason: err.Error()}
	}
	return governanceports.Decision{Allow: true, Reason: "executable allowed"}
}

func (e *permissionManagerEnforcer) checkNetwork(ctx context.Context, agentID, resource string) governanceports.Decision {
	if err := e.pm.CheckNetwork(ctx, agentID, "egress", "tcp", resource, 0); err != nil {
		return governanceports.Decision{Allow: false, Reason: err.Error()}
	}
	return governanceports.Decision{Allow: true, Reason: "network allowed"}
}

func (e *permissionManagerEnforcer) checkTool(ctx context.Context, agentID, toolName string) governanceports.Decision {
	if err := e.pm.AuthorizeTool(ctx, agentID, nil, nil); err != nil {
		return governanceports.Decision{Allow: false, Reason: err.Error()}
	}
	return governanceports.Decision{Allow: true, Reason: "tool allowed"}
}

func (e *permissionManagerEnforcer) checkCapability(ctx context.Context, agentID, capability string) governanceports.Decision {
	if err := e.pm.CheckCapability(ctx, agentID, capability); err != nil {
		return governanceports.Decision{Allow: false, Reason: err.Error()}
	}
	return governanceports.Decision{Allow: true, Reason: "capability allowed"}
}

func (e *permissionManagerEnforcer) checkIPC(ctx context.Context, agentID, resource string) governanceports.Decision {
	if err := e.pm.CheckIPC(ctx, agentID, "ipc", resource); err != nil {
		return governanceports.Decision{Allow: false, Reason: err.Error()}
	}
	return governanceports.Decision{Allow: true, Reason: "ipc allowed"}
}
