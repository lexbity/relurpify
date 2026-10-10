package authorization

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/governance/policy"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
)

const commandApprovalAction = "command:exec"

// Command-decision reason codes for the paths that escalate before/around the
// pattern decision.
const (
	commandReasonParseFailed = "parse_failed"
	commandReasonOpaque      = "opaque_wrapper"
)

// BashConfig holds the allow/deny patterns and default decision for
// bash command authorization. Default is a typed decision; its zero value
// resolves to ask (never allow).
type BashConfig struct {
	AllowPatterns []string
	DenyPatterns  []string
	Default       permissions.Decision
}

// CommandAuthorizationRequest describes a command that should be validated
// against executable permissions and manifest bash policy.
type CommandAuthorizationRequest struct {
	Command []string
	Env     []string
	Source  string
}

// AuthorizeCommand centralizes runtime command authorization so all wrappers
// share the same executable, bash policy, and HITL approval behavior.
//
// Lifting is total: every path ends in allow, ask, or deny. A command that
// cannot be parsed, or that uses a dynamic/opaque constructor, escalates to ask
// rather than skipping its checks (P-3).
func AuthorizeCommand(ctx context.Context, manager *PermissionManager, agentID string, bashCfg *BashConfig, req CommandAuthorizationRequest) error {
	if len(req.Command) == 0 {
		return fmt.Errorf("command empty")
	}
	binary := req.Command[0]
	args := req.Command[1:]
	commandString := strings.TrimSpace(strings.Join(req.Command, " "))

	// 1. Unwrap transparent wrappers; record each step for forensics.
	_, steps, unwrapDynamic := UnwrapCommand(req.Command)
	emitUnwrapSteps(ctx, manager, agentID, commandString, steps)

	// 2. Static lifting is total. A parse failure becomes an ask; it never
	// falls through to weaker checks.
	lifted := &LiftedPermissions{}
	if err := liftCommand(req.Command, lifted, 0); err != nil {
		emitCommandEvent(ctx, manager, agentID, fwtelemetry.CommandEvent{
			Kind:    fwtelemetry.CommandEventParseFailed,
			Command: commandString,
			Reason:  classifyParseError(err),
		})
		return requireCommandApproval(ctx, manager, agentID, req, commandReasonParseFailed, commandString,
			"command could not be statically analyzed — approve to run unreviewed")
	}
	if unwrapDynamic || lifted.HasDynamic {
		emitCommandEvent(ctx, manager, agentID, fwtelemetry.CommandEvent{
			Kind:    fwtelemetry.CommandEventOpaqueConstructor,
			Command: commandString,
		})
		return requireCommandApproval(ctx, manager, agentID, req, commandReasonOpaque, commandString,
			"Command contains dynamic or opaque execution syntax (eval, command substitution, or a code-bearing wrapper)")
	}

	// 3. Lifted filesystem/executable/network checks for the command and every
	// transparently-unwrapped inner command.
	if manager != nil {
		if err := checkLifted(ctx, manager, agentID, commandBase(binary), args, req.Env, lifted); err != nil {
			return err
		}
	}

	// 4. Pattern decision with command-text glob semantics (D-8).
	if bashCfg == nil {
		return nil
	}
	decision := DecideCommandByPatterns(commandString, bashCfg.AllowPatterns, bashCfg.DenyPatterns, bashCfg.Default)
	switch decision.Decision {
	case permissions.DecisionDeny:
		return fmt.Errorf("command blocked: denied by bash_permissions")
	case permissions.DecisionAsk:
		return requireCommandApproval(ctx, manager, agentID, req, decision.Reason, commandString, "bash permission policy")
	default: // DecisionAllow
		return nil
	}
}

// checkLifted validates every filesystem, network, and executable permission a
// command's lifting produced. The base binary is checked with the request's
// environment so env predicates still apply to it.
func checkLifted(ctx context.Context, manager *PermissionManager, agentID, baseBinary string, baseArgs, baseEnv []string, lifted *LiftedPermissions) error {
	for _, fsPerm := range lifted.FileSystem {
		if err := manager.CheckFileAccess(ctx, agentID, fsPerm.Action, fsPerm.Path); err != nil {
			return fmt.Errorf("semantic filesystem check denied: %w", err)
		}
	}
	for _, netPerm := range lifted.Network {
		target, err := manager.ResolveTarget(ctx, netPerm.Host)
		if err != nil {
			return fmt.Errorf("semantic network check denied: %w", err)
		}
		if err := manager.CheckNetwork(ctx, agentID, netPerm.Direction, netPerm.Protocol, target, netPerm.Port); err != nil {
			return fmt.Errorf("semantic network check denied: %w", err)
		}
	}

	seen := make(map[string]struct{}, len(lifted.Executables))
	for _, execPerm := range lifted.Executables {
		key := execPerm.Binary + "\x00" + strings.Join(execPerm.Args, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		args := execPerm.Args
		var env []string
		if execPerm.Binary == baseBinary {
			args = baseArgs
			env = baseEnv
		}
		if err := manager.CheckExecutable(ctx, agentID, execPerm.Binary, args, env); err != nil {
			return fmt.Errorf("semantic executable check denied: %w", err)
		}
	}
	if _, ok := seen[baseBinary+"\x00"+strings.Join(baseArgs, "\x00")]; !ok {
		if err := manager.CheckExecutable(ctx, agentID, baseBinary, baseArgs, baseEnv); err != nil {
			return err
		}
	}
	return nil
}

// requireCommandApproval routes a command escalation (parse failure, opaque
// constructor, or ask default) through HITL with a typed reason.
func requireCommandApproval(ctx context.Context, manager *PermissionManager, agentID string, req CommandAuthorizationRequest, reason, commandString, justification string) error {
	if manager == nil {
		return fmt.Errorf("command blocked: approval required but permission manager missing")
	}
	metadata := map[string]string{"reason": reason}
	if source := strings.TrimSpace(req.Source); source != "" {
		metadata["source"] = source
	}
	return manager.RequireApproval(ctx, agentID, ucperms.PermissionDescriptor{
		Type:         ucperms.PermissionTypeHITL,
		Action:       commandApprovalAction,
		Resource:     commandString,
		Metadata:     metadata,
		RequiresHITL: true,
	}, justification, policy.GrantScopeOneTime, policy.RiskLevelMedium, 0)
}

// emitUnwrapSteps records each transparent-wrapper unwrapping.
func emitUnwrapSteps(ctx context.Context, manager *PermissionManager, agentID, command string, steps []UnwrapStep) {
	if manager == nil {
		return
	}
	for _, step := range steps {
		manager.emitCommandEvent(ctx, agentID, fwtelemetry.CommandEvent{
			Kind:    fwtelemetry.CommandEventWrapperUnwrapped,
			Command: command,
			Wrapper: step.Wrapper,
			Depth:   step.Depth,
		})
	}
}

// emitCommandEvent records a single command forensic signal.
func emitCommandEvent(ctx context.Context, manager *PermissionManager, agentID string, event fwtelemetry.CommandEvent) {
	if manager == nil {
		return
	}
	manager.emitCommandEvent(ctx, agentID, event)
}

// classifyParseError maps a parser error to the telemetry reason class without
// echoing raw parser text to the model.
func classifyParseError(err error) string {
	if err != nil {
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "depth") || strings.Contains(msg, "too deep"):
			return "depth"
		case strings.Contains(msg, "lexer") || strings.Contains(msg, "invalid"):
			return "lexer"
		default:
			return "syntax"
		}
	}
	return ""
}
