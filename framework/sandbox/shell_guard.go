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

// ShellGuard acts as a command policy wrapper for shell blacklist and HITL
// escalation. It still implements CommandRunner for compatibility, but the
// policy check is now a separate step that can be composed around any runner.
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

// AllowCommand implements CommandPolicy.
func (g *ShellGuard) AllowCommand(ctx context.Context, req CommandRequest) error {
	if g == nil {
		return fmt.Errorf("shell guard missing")
	}
	// Reconstruct the command string as "binary arg1 arg2 ..."
	cmdStr := strings.Join(req.Args, " ")
	if rule := g.blacklist.Check(cmdStr); rule != nil {
		switch rule.Action {
		case BlacklistActionBlock:
			return fmt.Errorf("shell filter blocked [%s]: %s", rule.ID, rule.Reason)
		case BlacklistActionHITL:
			if g.manager != nil {
				approved, err := g.manager.ApproveCommand(ctx, g.agentID, rule.ID, rule.Reason, cmdStr)
				if err != nil {
					return fmt.Errorf("shell filter HITL error [%s]: %w", rule.ID, err)
				}
				if !approved {
					return fmt.Errorf("shell filter denied [%s]: %s", rule.ID, rule.Reason)
				}
				return nil
			}
			return fmt.Errorf("shell filter requires HITL approval [%s]: %s (no approver configured)", rule.ID, rule.Reason)
		}
	}
	return nil
}

// Run implements CommandRunner. It first applies the shell policy, then
// forwards to the underlying runner.
func (g *ShellGuard) Run(ctx context.Context, req CommandRequest) (string, string, error) {
	if g == nil || g.inner == nil {
		return "", "", fmt.Errorf("shell guard missing")
	}
	if err := g.AllowCommand(ctx, req); err != nil {
		return "", "", err
	}
	return g.inner.Run(ctx, req)
}

var _ CommandPolicy = (*ShellGuard)(nil)
