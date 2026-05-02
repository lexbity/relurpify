package authorization

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

const commandApprovalAction = "command:exec"

// CommandAuthorizationRequest describes a command that should be validated
// against executable permissions and manifest bash policy.
type CommandAuthorizationRequest struct {
	Command []string
	Env     []string
	Source  string
}

// AuthorizeCommand centralizes runtime command authorization so all wrappers
// share the same executable, bash policy, and HITL approval behavior.
func AuthorizeCommand(ctx context.Context, manager *PermissionManager, agentID string, spec *agentspec.AgentRuntimeSpec, req CommandAuthorizationRequest) error {
	if len(req.Command) == 0 {
		return fmt.Errorf("command empty")
	}
	binary := req.Command[0]
	args := []string{}
	if len(req.Command) > 1 {
		args = req.Command[1:]
	}
	if manager != nil {
		if err := manager.CheckExecutable(ctx, agentID, binary, args, req.Env); err != nil {
			return err
		}
	}
	if spec == nil {
		return nil
	}
	commandString := strings.TrimSpace(binary + " " + strings.Join(args, " "))
	decision := decideCommandByPatterns(commandString, spec.Bash.AllowPatterns, spec.Bash.DenyPatterns, spec.Bash.Default)
	switch decision {
	case agentspec.AgentPermissionDeny:
		return fmt.Errorf("command blocked: denied by bash_permissions")
	case agentspec.AgentPermissionAsk:
		if manager == nil {
			return fmt.Errorf("command blocked: approval required but permission manager missing")
		}
		metadata := map[string]string{}
		if source := strings.TrimSpace(req.Source); source != "" {
			metadata["source"] = source
		}
		return manager.RequireApproval(ctx, agentID, contracts.PermissionDescriptor{
			Type:         contracts.PermissionTypeHITL,
			Action:       commandApprovalAction,
			Resource:     commandString,
			Metadata:     metadata,
			RequiresHITL: true,
		}, "bash permission policy", GrantScopeOneTime, RiskLevelMedium, 0)
	default:
		return nil
	}
}

func decideCommandByPatterns(target string, allowPatterns, denyPatterns []string, defaultDecision agentspec.AgentPermissionLevel) agentspec.AgentPermissionLevel {
	target = strings.TrimSpace(target)
	for _, pattern := range denyPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if matchGlob(pattern, target) {
			return agentspec.AgentPermissionDeny
		}
	}
	for _, pattern := range allowPatterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if matchGlob(pattern, target) {
			return agentspec.AgentPermissionAllow
		}
	}
	if defaultDecision == "" {
		return agentspec.AgentPermissionAllow
	}
	return defaultDecision
}
