package authorization

import (
	"context"
	"fmt"
	"path/filepath"
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

	// 1. Perform static semantic lifting checks (Stage 1)
	cmdStr := extractShellCommandString(binary, args)
	lifted, err := LiftShellCommand(cmdStr)
	if err == nil {
		if lifted.HasDynamic {
			if manager == nil {
				return fmt.Errorf("dynamic shell command execution blocked: permission manager missing")
			}
			return manager.RequireApproval(ctx, agentID, contracts.PermissionDescriptor{
				Type:         contracts.PermissionTypeHITL,
				Action:       "command:dynamic",
				Resource:     cmdStr,
				RequiresHITL: true,
			}, "Command contains dynamic execution syntax (eval, command substitution, or backticks)", GrantScopeOneTime, RiskLevelHigh, 0)
		}

		if manager != nil {
			// Validate FS virtual permissions
			for _, fsPerm := range lifted.FileSystem {
				if err := manager.CheckFileAccess(ctx, agentID, fsPerm.Action, fsPerm.Path); err != nil {
					return fmt.Errorf("semantic filesystem check denied: %w", err)
				}
			}
			// Validate Executable virtual permissions
			for _, execPerm := range lifted.Executables {
				if err := manager.CheckExecutable(ctx, agentID, execPerm.Binary, execPerm.Args, nil); err != nil {
					return fmt.Errorf("semantic executable check denied: %w", err)
				}
			}
			// Validate Network virtual permissions
			for _, netPerm := range lifted.Network {
				if err := manager.CheckNetwork(ctx, agentID, netPerm.Direction, netPerm.Protocol, netPerm.Host, netPerm.Port); err != nil {
					return fmt.Errorf("semantic network check denied: %w", err)
				}
			}
		}
	}

	// 2. Fallback to base executable validation
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

// extractShellCommandString normalizes shell wrappers and returns the raw command string.
func extractShellCommandString(binary string, args []string) string {
	base := strings.ToLower(filepath.Base(binary))
	if base == "sh" || base == "bash" || base == "zsh" || base == "dash" {
		for i := 0; i < len(args)-1; i++ {
			if args[i] == "-c" {
				return args[i+1]
			}
		}
	}
	return strings.Join(append([]string{binary}, args...), " ")
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
