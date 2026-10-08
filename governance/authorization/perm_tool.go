package authorization

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// toolLike is the narrow tool surface needed by permission authorization.
// It deliberately exposes the permission set as the canonical
// *permissions.PermissionSet rather than a capability-owned wrapper type so a
// foreign tool can satisfy it structurally without governance importing the
// capability domain (which would invert the allowed package direction).
type toolLike interface {
	Name() string
	Tags() []string
	PermissionSet() *permissions.PermissionSet
}

// toolAdapter wraps a foreign tool implementation to satisfy authorization.Tool.
type toolAdapter struct {
	inner toolLike
}

func (a *toolAdapter) Name() string   { return a.inner.Name() }
func (a *toolAdapter) Tags() []string { return a.inner.Tags() }
func (a *toolAdapter) Permissions() ToolPermissions {
	return ToolPermissions{
		Permissions: a.inner.PermissionSet(),
	}
}

// AuthorizeTool ensures the tool requirements fit the declared permissions.
// Undeclared permissions are handled according to the configured default
// decision: ask (the terminal default) routes to HITL, deny returns an error.
// Allow is rejected at registration and cannot be configured.
func (m *PermissionManager) AuthorizeTool(ctx context.Context, agentID string, tool any, args map[string]any) error {
	if m == nil || tool == nil {
		return errors.New("permission manager or tool missing")
	}
	t, ok := tool.(Tool)
	if !ok {
		pt, ok2 := tool.(toolLike)
		if !ok2 {
			return errors.New("tool does not implement authorization.Tool or the required tool surface")
		}
		t = &toolAdapter{inner: pt}
	}
	if m.toolAllowedByTaskGrant(ctx, t) {
		m.log(ctx, agentID, toolDescriptor(t.Name(), agentID), "tool_allowed_task_grant", map[string]any{"tags": t.Tags()})
		return nil
	}
	requirements := t.Permissions()
	if err := requirements.Validate(); err != nil {
		return fmt.Errorf("tool %s permission invalid: %w", t.Name(), err)
	}
	if undeclared := m.collectUndeclared(requirements.Permissions); len(undeclared) > 0 {
		if err := m.handleUndeclaredTool(ctx, agentID, t.Name(), undeclared); err != nil {
			return err
		}
	}
	m.log(ctx, agentID, toolDescriptor(t.Name(), agentID), "tool_allowed", nil)
	return nil
}

// AuthorizeToolByName authorizes a tool referenced only by name, without its
// declared permission set. Because the tool's requirements are unknown, the
// request is treated as fully undeclared and governed by the configured default
// policy (Ask by default), so callers that cannot supply the tool fail closed.
func (m *PermissionManager) AuthorizeToolByName(ctx context.Context, agentID, toolName string) error {
	if m == nil {
		return errors.New("permission manager missing")
	}
	name := strings.TrimSpace(toolName)
	if name == "" {
		return errors.New("tool name required")
	}
	// A task grant is tag-scoped; a name-only reference carries no tags, so it
	// cannot match one.
	if err := m.handleUndeclaredTool(ctx, agentID, name, []string{"tool permissions unknown"}); err != nil {
		return err
	}
	m.log(ctx, agentID, toolDescriptor(name, agentID), "tool_allowed", nil)
	return nil
}

// handleUndeclaredTool applies the configured default policy to a tool whose
// permissions are not covered by the agent's declared set.
func (m *PermissionManager) handleUndeclaredTool(ctx context.Context, agentID, name string, undeclared []string) error {
	desc := toolDescriptor(name, agentID)
	switch m.effectiveDefaultDecision() {
	case permissions.DecisionDeny:
		return m.deny(ctx, agentID, desc, "tool exceeds declared permissions")
	default: // DecisionAsk
		desc.RequiresHITL = true
		m.emitPolicyDecision(ctx, agentID, desc, fwtelemetry.PolicyEffectRequireApproval, "undeclared permissions require approval", map[string]any{"undeclared": undeclared})
		return m.RequireApproval(ctx, agentID, desc,
			fmt.Sprintf("tool %s requires: %s", name, strings.Join(undeclared, ", ")),
			policy.GrantScopeSession, policy.RiskLevelMedium, 0)
	}
}

// toolDescriptor builds the canonical permission descriptor for a tool action.
func toolDescriptor(name, agentID string) permissions.PermissionDescriptor {
	return permissions.PermissionDescriptor{
		Type:     permissions.PermissionTypeHITL,
		Action:   fmt.Sprintf("tool:%s", name),
		Resource: agentID,
	}
}

// collectUndeclared returns human-readable descriptions of any permissions
// required by the tool that are not covered by the agent manifest.
func (m *PermissionManager) collectUndeclared(requirements *permissions.PermissionSet) []string {
	var missing []string
	for _, perm := range requirements.FileSystem {
		if m.findFilesystemPermission(perm.Action, perm.Path) == nil {
			missing = append(missing, fmt.Sprintf("fs %s %s", perm.Action, perm.Path))
		}
	}
	for _, exec := range requirements.Executables {
		if m.findExecutablePermission(exec.Binary) == nil {
			missing = append(missing, fmt.Sprintf("exec %s", exec.Binary))
		}
	}
	for _, net := range requirements.Network {
		if m.findNetworkPermission(net.Direction, net.Protocol, net.Host, net.Port) == nil {
			missing = append(missing, fmt.Sprintf("net %s %s", net.Direction, net.Host))
		}
	}
	return missing
}
