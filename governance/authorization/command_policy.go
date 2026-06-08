package authorization

import (
	"context"
	"strings"
)

// CommandAuthorizationPolicy adapts PermissionManager command checks to the
// sandbox command policy interface.
type CommandAuthorizationPolicy struct {
	manager *PermissionManager
	agentID string
	bashCfg *BashConfig
	source  string
}

// NewCommandAuthorizationPolicy creates a sandbox policy adapter backed by the
// current authorization state.
func NewCommandAuthorizationPolicy(manager *PermissionManager, agentID string, bashCfg *BashConfig, source string) *CommandAuthorizationPolicy {
	return &CommandAuthorizationPolicy{
		manager: manager,
		agentID: agentID,
		bashCfg: bashCfg,
		source:  source,
	}
}

// CheckCommand verifies a command against the authorization policy.
func (p *CommandAuthorizationPolicy) CheckCommand(ctx context.Context, args, env []string) error {
	reqSpec := CommandAuthorizationRequest{
		Command: append([]string(nil), args...),
		Env:     append([]string(nil), env...),
		Source:  strings.TrimSpace(p.source),
	}
	return AuthorizeCommand(ctx, p.manager, p.agentID, p.bashCfg, reqSpec)
}
