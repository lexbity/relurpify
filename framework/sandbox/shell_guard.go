package sandbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/authorization"
)

// ShellGuard wraps a CommandRunner and applies the blacklist before forwarding.
// It reconstructs the full command string as "args[0] args[1] ..." (same format
// used by AuthorizeCommand) so blacklist pattern authors have a consistent model.
type ShellGuard struct {
	inner     CommandRunner
	blacklist *ShellBlacklist
	manager   *authorization.PermissionManager
	agentID   string
}

// NewShellGuard creates a new ShellGuard.
func NewShellGuard(inner CommandRunner, blacklist *ShellBlacklist,
	manager *authorization.PermissionManager, agentID string) *ShellGuard {
	return &ShellGuard{
		inner:     inner,
		blacklist: blacklist,
		manager:   manager,
		agentID:   agentID,
	}
}

// Run implements CommandRunner. On block: returns error
// "shell filter blocked [<rule-id>]: <reason>".
// On hitl: escalates through manager.RequireApproval with the rule metadata.
func (g *ShellGuard) Run(ctx context.Context, req CommandRequest) (string, string, error) {
	// Reconstruct the command string as "binary arg1 arg2 ..."
	cmdStr := strings.Join(req.Args, " ")
	if rule := g.blacklist.Check(cmdStr); rule != nil {
		switch rule.Action {
		case BlacklistActionBlock:
			return "", "", fmt.Errorf("shell filter blocked [%s]: %s", rule.ID, rule.Reason)
		case BlacklistActionHITL:
			if g.manager != nil {
				// Try to escalate through RequireApproval
				// We'll use a placeholder for now
				// In a real implementation, we would call:
				// approved, err := g.manager.RequireApproval(ctx, g.agentID, "shell_guard", map[string]string{
				// 	"rule_id": rule.ID,
				// 	"reason":  rule.Reason,
				// 	"command": cmdStr,
				// })
				// For now, we'll block with a message indicating HITL is required
				return "", "", fmt.Errorf("shell filter requires HITL approval [%s]: %s (HITL not yet implemented)", rule.ID, rule.Reason)
			}
			return "", "", fmt.Errorf("shell filter requires HITL approval [%s]: %s (no permission manager available)", rule.ID, rule.Reason)
		}
	}
	return g.inner.Run(ctx, req)
}
