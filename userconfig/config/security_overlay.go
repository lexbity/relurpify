package config

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/userconfig/config/security"
)

// OverlaySecurityBundle folds the split security bundle (LocalTool, Shell,
// Sandbox) onto a base EffectiveAgentContract, returning a new contract with
// overlaid values. It is pure: the base contract is not mutated.
//
// LocalTool execute policies override the spec's ToolExecutionPolicy defaults.
// Shell deny patterns (action=block) are appended to Bash.DenyPatterns.
// Sandbox ReadOnlyRoot and NoNewPrivileges override SecuritySpec.
func OverlaySecurityBundle(base *EffectiveAgentContract, bundle *security.Bundle) (*EffectiveAgentContract, error) {
	if base == nil {
		return nil, fmt.Errorf("base contract required")
	}
	if bundle == nil {
		return BuildEffectiveAgentContract(
			base.AgentID,
			overlayAgentSpec(base.AgentSpec, nil),
			base.Permissions,
			base.Resources,
			base.Security,
			base.Sources,
		), nil
	}

	return BuildEffectiveAgentContract(
		base.AgentID,
		overlayAgentSpec(base.AgentSpec, bundle),
		base.Permissions,
		base.Resources,
		overlaySecurity(base.Security, bundle),
		base.Sources,
	), nil
}

func overlayAgentSpec(base *AgentSpec, bundle *security.Bundle) *AgentSpec {
	if base == nil {
		return nil
	}

	out := &AgentSpec{
		Implementation:      base.Implementation,
		Version:             base.Version,
		Prompt:              base.Prompt,
		Model:               base.Model,
		ToolExecutionPolicy: make(map[string]ToolPolicy, len(base.ToolExecutionPolicy)),
		ProviderPolicies:    make(map[string]ProviderPolicy, len(base.ProviderPolicies)),
		Bash: AgentBashPermissions{
			AllowPatterns: append([]string(nil), base.Bash.AllowPatterns...),
			DenyPatterns:  append([]string(nil), base.Bash.DenyPatterns...),
			Default:       base.Bash.Default,
		},
		NativeToolCalling: base.NativeToolCalling,
		Logging:          base.Logging,
	}

	for k, v := range base.ToolExecutionPolicy {
		out.ToolExecutionPolicy[k] = v
	}
	for k, v := range base.ProviderPolicies {
		out.ProviderPolicies[k] = v
	}

	if bundle == nil {
		return out
	}

	// Overlay LocalTool execute policy onto ToolExecutionPolicy.
	for name, toolPol := range bundle.LocalTool {
		exec := strings.TrimSpace(toolPol.Execute)
		if exec != "" {
			out.ToolExecutionPolicy[name] = ToolPolicy{
				Execute: AgentPermissionLevel(exec),
			}
		}
	}

	// Overlay Shell deny patterns (block action) onto Bash.DenyPatterns.
	if bundle.Shell != nil {
		for _, rule := range bundle.Shell.Rules {
			if strings.ToLower(strings.TrimSpace(rule.Action)) == "block" && strings.TrimSpace(rule.Pattern) != "" {
				out.Bash.DenyPatterns = append(out.Bash.DenyPatterns, strings.TrimSpace(rule.Pattern))
			}
		}
	}

	return out
}

func overlaySecurity(base SecuritySpec, bundle *security.Bundle) SecuritySpec {
	out := base
	if bundle.Sandbox != nil {
		out.ReadOnlyRoot = bundle.Sandbox.ReadOnlyRoot
		out.NoNewPrivileges = bundle.Sandbox.NoNewPrivileges
	}
	return out
}
