package authorization

import (
	"context"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
)

// CommandAuthorizationPolicy adapts PermissionManager command checks to the
// sandbox command policy interface.
type CommandAuthorizationPolicy struct {
	manager *PermissionManager
	agentID string
	spec    *agentspec.AgentRuntimeSpec
	source  string
}

// NewCommandAuthorizationPolicy creates a sandbox policy adapter backed by the
// current authorization state.
func NewCommandAuthorizationPolicy(manager *PermissionManager, agentID string, spec *agentspec.AgentRuntimeSpec, source string) sandbox.CommandPolicy {
	return CommandAuthorizationPolicy{
		manager: manager,
		agentID: agentID,
		spec:    spec,
		source:  source,
	}
}

// AllowCommand implements sandbox.CommandPolicy.
func (p CommandAuthorizationPolicy) AllowCommand(ctx context.Context, req sandbox.CommandRequest) error {
	reqSpec := CommandAuthorizationRequest{
		Command: append([]string(nil), req.Args...),
		Env:     append([]string(nil), req.Env...),
		Source:  strings.TrimSpace(p.source),
	}
	return AuthorizeCommand(ctx, p.manager, p.agentID, p.spec, reqSpec)
}
