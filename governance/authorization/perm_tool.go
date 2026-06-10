package authorization

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
)

// AuthorizeTool ensures the tool requirements fit the declared permissions.
// Undeclared permissions are handled according to the configured defaultPolicy:
// Ask (default) routes to HITL, Allow proceeds, Deny returns an error.
func (m *PermissionManager) AuthorizeTool(ctx context.Context, agentID string, tool any, args map[string]interface{}) error {
	if m == nil || tool == nil {
		return errors.New("permission manager or tool missing")
	}
	t, ok := tool.(Tool)
	if !ok {
		pt, ok2 := tool.(ports.Tool)
		if !ok2 {
			return errors.New("tool does not implement authorization.Tool or ports.Tool")
		}
		t = ToolFromPorts(pt)
	}
	if m.toolAllowedByTaskGrant(ctx, t) {
		desc := permissions.PermissionDescriptor{
			Type:     permissions.PermissionTypeHITL,
			Action:   fmt.Sprintf("tool:%s", t.Name()),
			Resource: agentID,
		}
		m.log(ctx, agentID, desc, "tool_allowed_task_grant", map[string]interface{}{"tags": t.Tags()})
		m.emitPolicyDecision(ctx, desc, "allow", "task grant matched tool tags", map[string]interface{}{"tags": t.Tags()})
		return nil
	}
	requirements := t.Permissions()
	if err := requirements.Validate(); err != nil {
		return fmt.Errorf("tool %s permission invalid: %w", t.Name(), err)
	}
	undeclared := m.collectUndeclared(requirements.Permissions)
	if len(undeclared) > 0 {
		switch m.effectiveDefaultPolicy() {
		case "deny":
			m.emitPolicyDecision(ctx, permissions.PermissionDescriptor{
				Type:     permissions.PermissionTypeHITL,
				Action:   fmt.Sprintf("tool:%s", t.Name()),
				Resource: agentID,
			}, "deny", "tool exceeds declared permissions", map[string]interface{}{"undeclared": undeclared})
			return fmt.Errorf("tool %s exceeds agent permissions: %s", t.Name(), strings.Join(undeclared, "; "))
		default: // "ask"
			m.emitPolicyDecision(ctx, permissions.PermissionDescriptor{
				Type:         permissions.PermissionTypeHITL,
				Action:       fmt.Sprintf("tool:%s", t.Name()),
				Resource:     agentID,
				RequiresHITL: true,
			}, "require_approval", "undeclared permissions require approval", map[string]interface{}{"undeclared": undeclared})
			if err := m.RequireApproval(ctx, agentID, permissions.PermissionDescriptor{
				Type:         permissions.PermissionTypeHITL,
				Action:       fmt.Sprintf("tool:%s", t.Name()),
				Resource:     agentID,
				RequiresHITL: true,
			}, fmt.Sprintf("tool %s requires: %s", t.Name(), strings.Join(undeclared, ", ")),
				policy.GrantScopeSession, policy.RiskLevelMedium, 0); err != nil {
				return err
			}
		}
	}
	desc := permissions.PermissionDescriptor{
		Type:     permissions.PermissionTypeHITL,
		Action:   fmt.Sprintf("tool:%s", t.Name()),
		Resource: agentID,
	}
	m.log(ctx, agentID, desc, "tool_allowed", nil)
	m.emitPolicyDecision(ctx, desc, "allow", "tool authorized", nil)
	return nil
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
