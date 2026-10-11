package react

import (
	"context"
	"sort"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	capability "codeburg.org/lexbit/relurpify/capability/registry"
	capresult "codeburg.org/lexbit/relurpify/capability/result"
	"codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/governance/policyresolve"
)

const (
	contextmgrPhaseExplore = "explore"
	contextmgrPhaseVerify  = "verify"
	contextmgrPhaseEdit    = "edit"
)

// availableTools returns the agent's tool list: the scoped registry's
// callable surface, deterministically ordered (D6). The step tool list is
// exactly (declared step scope) ∩ (callable) ∩ (available at invocation) —
// no prompt-side phase/mode hiding may participate, because a filter here is
// how a declared repair scope silently ran disarmed (the live scratch-turn
// finding). Phase transitions remain context management
// (react_observe_node), and enforcement stays at invocation.
func (a *ReActAgent) availableTools(ctx context.Context) []ports.Tool {
	if a.executionCapabilityCatalog(ctx) == nil && a.Tools == nil {
		return nil
	}
	tools := executionCallableTools(ctx, a.Tools, a.executionCapabilityCatalog(ctx))
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name() < tools[j].Name() })
	return tools
}

func (a *ReActAgent) executionCapabilityCatalog(ctx context.Context) *capability.ExecutionCapabilityCatalogSnapshot {
	if a == nil {
		return nil
	}
	if a.executionCatalog != nil {
		return a.executionCatalog
	}
	if a.Tools == nil {
		return nil
	}
	return a.Tools.CaptureExecutionCatalogSnapshot(ctx)
}

func (a *ReActAgent) executionPolicySnapshot(ctx context.Context) *capresult.PolicySnapshot {
	if catalog := a.executionCapabilityCatalog(ctx); catalog != nil {
		return catalog.PolicySnapshot()
	}
	if a == nil || a.Tools == nil {
		return nil
	}
	return a.Tools.CapturePolicySnapshot()
}

func (a *ReActAgent) executionCapabilityDescriptor(ctx context.Context, idOrName string) (descriptor.CapabilityDescriptor, bool) {
	if catalog := a.executionCapabilityCatalog(ctx); catalog != nil {
		if entry, ok := catalog.GetCapability(idOrName); ok {
			return entry.Descriptor, true
		}
	}
	if a == nil || a.Tools == nil {
		return descriptor.CapabilityDescriptor{}, false
	}
	return a.Tools.GetCapability(idOrName)
}

func executionCallableTools(ctx context.Context, registry *capability.CapabilityRegistry, catalog *capability.ExecutionCapabilityCatalogSnapshot) []ports.Tool {
	if catalog != nil {
		return catalog.ModelCallableTools(ctx)
	}
	if registry == nil {
		return nil
	}
	return registry.ModelCallableTools(ctx)
}

func (a *ReActAgent) resolvedAgentPolicy() policy.ResolvedAgentPolicy {
	if a == nil || a.Config == nil || a.Config.AgentSpec == nil {
		return policy.ResolvedAgentPolicy{}
	}
	reg := capability.NewPolicyResolveRegistry(a.Tools)
	cfg := agentspec.ToPolicyResolveOrchConfig(a.Config.AgentSpec.Orchestration)
	return policyresolve.ResolveAgentPolicy(reg, cfg)
}

func (a *ReActAgent) recoveryProbeTools() []string {
	resolved := a.resolvedAgentPolicy()
	return append([]string{}, resolved.RecoveryProbeCapabilities...)
}

func (a *ReActAgent) verificationSuccessTools() []string {
	resolved := a.resolvedAgentPolicy()
	return append([]string{}, resolved.VerificationSuccessCapabilities...)
}
