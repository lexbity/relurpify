package sandbox

import (
	"context"
	"fmt"
	"strings"
)

// CommandApprover is the interface ShellGuard needs for HITL escalation of
// blacklist-matched commands. authorization.PermissionManager satisfies this
// via a thin adapter at the wiring site, breaking the import cycle between
// framework/sandbox and framework/authorization.
type CommandApprover interface {
	ApproveCommand(ctx context.Context, agentID, ruleID, reason, command string) (bool, error)
}

// ShellGuard wraps a CommandRunner and applies the blacklist before forwarding.
// It reconstructs the full command string as "args[0] args[1] ..." (same format
// used by AuthorizeCommand) so blacklist pattern authors have a consistent model.
type ShellGuard struct {
	inner     CommandRunner
	blacklist *ShellBlacklist
	manager   CommandApprover
	agentID   string
}

// NewShellGuard creates a new ShellGuard.
func NewShellGuard(inner CommandRunner, blacklist *ShellBlacklist,
	manager CommandApprover, agentID string) *ShellGuard {
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
				approved, err := g.manager.ApproveCommand(ctx, g.agentID, rule.ID, rule.Reason, cmdStr)
				if err != nil {
					return "", "", fmt.Errorf("shell filter HITL error [%s]: %w", rule.ID, err)
				}
				if !approved {
					return "", "", fmt.Errorf("shell filter denied [%s]: %s", rule.ID, rule.Reason)
				}
				break
			}
			return "", "", fmt.Errorf("shell filter requires HITL approval [%s]: %s (no approver configured)", rule.ID, rule.Reason)
		}
	}
	return g.inner.Run(ctx, req)
}
