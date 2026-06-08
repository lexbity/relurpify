package agentspec

import (
	"codeburg.org/lexbit/relurpify/governance/ports"
)

func (a *AgentRuntimeSpec) GetAllowedCapabilities() []ports.CapabilitySelectorView {
	if a == nil {
		return nil
	}
	out := make([]ports.CapabilitySelectorView, len(a.AllowedCapabilities))
	for i, cs := range a.AllowedCapabilities {
		out[i] = cs.ToView()
	}
	return out
}

func (a *AgentRuntimeSpec) GetToolExecutionPolicy() map[string]ports.ToolPolicyView {
	if a == nil {
		return nil
	}
	out := make(map[string]ports.ToolPolicyView, len(a.ToolExecutionPolicy))
	for k, tp := range a.ToolExecutionPolicy {
		out[k] = ports.ToolPolicyView{Execute: string(tp.Execute)}
	}
	return out
}

func (a *AgentRuntimeSpec) GetCapabilityPolicies() []ports.CapabilityPolicyView {
	if a == nil {
		return nil
	}
	out := make([]ports.CapabilityPolicyView, len(a.CapabilityPolicies))
	for i, cp := range a.CapabilityPolicies {
		out[i] = ports.CapabilityPolicyView{
			Selector: cp.Selector.ToView(),
			Execute:  string(cp.Execute),
		}
	}
	return out
}

func (a *AgentRuntimeSpec) GetSessionPolicies() []ports.SessionPolicyView {
	if a == nil {
		return nil
	}
	out := make([]ports.SessionPolicyView, len(a.SessionPolicies))
	for i, sp := range a.SessionPolicies {
		out[i] = sp.ToView()
	}
	return out
}

func (a *AgentRuntimeSpec) GetProviderPolicies() map[string]ports.ProviderPolicyView {
	if a == nil {
		return nil
	}
	out := make(map[string]ports.ProviderPolicyView, len(a.ProviderPolicies))
	for k, pp := range a.ProviderPolicies {
		out[k] = ports.ProviderPolicyView{
			Activate:             string(pp.Activate),
			DefaultTrust:         string(pp.DefaultTrust),
			AllowCredentialShare: pp.AllowCredentialSharing,
		}
	}
	return out
}

func (a *AgentRuntimeSpec) GetGlobalPolicies() map[string]string {
	if a == nil {
		return nil
	}
	out := make(map[string]string, len(a.GlobalPolicies))
	for k, v := range a.GlobalPolicies {
		out[k] = string(v)
	}
	return out
}

func (a *AgentRuntimeSpec) GetBrowser() ports.BrowserSpecView {
	if a == nil || a.Browser == nil {
		return ports.BrowserSpecView{}
	}
	actions := make(map[string]string, len(a.Browser.Actions))
	for k, v := range a.Browser.Actions {
		actions[k] = string(v)
	}
	return ports.BrowserSpecView{
		Enabled:         a.Browser.Enabled,
		DefaultBackend:  a.Browser.DefaultBackend,
		AllowedBackends: append([]string{}, a.Browser.AllowedBackends...),
		Actions:         actions,
	}
}

func (a *AgentRuntimeSpec) GetOrchestration() ports.OrchestrationConfigView {
	if a == nil {
		return ports.OrchestrationConfigView{}
	}
	pcs := make(map[string][]ports.CapabilitySelectorView, len(a.Orchestration.PhaseCapabilitySelectors))
	for k, selectors := range a.Orchestration.PhaseCapabilitySelectors {
		views := make([]ports.CapabilitySelectorView, len(selectors))
		for j, cs := range selectors {
			views[j] = cs.ToView()
		}
		pcs[k] = views
	}
	pc := make(map[string][]string, len(a.Orchestration.PhaseCapabilities))
	for k, caps := range a.Orchestration.PhaseCapabilities {
		pc[k] = append([]string{}, caps...)
	}
	return ports.OrchestrationConfigView{
		PhaseCapabilities:        pc,
		PhaseCapabilitySelectors: pcs,
	}
}

func (cs CapabilitySelector) ToView() ports.CapabilitySelectorView {
	runtimeFamilies := make([]string, len(cs.RuntimeFamilies))
	for i, rf := range cs.RuntimeFamilies {
		runtimeFamilies[i] = string(rf)
	}
	trustClasses := make([]string, len(cs.TrustClasses))
	for i, tc := range cs.TrustClasses {
		trustClasses[i] = string(tc)
	}
	riskClasses := make([]string, len(cs.RiskClasses))
	for i, rc := range cs.RiskClasses {
		riskClasses[i] = string(rc)
	}
	effectClasses := make([]string, len(cs.EffectClasses))
	for i, ec := range cs.EffectClasses {
		effectClasses[i] = string(ec)
	}
	roles := make([]string, len(cs.CoordinationRoles))
	for i, r := range cs.CoordinationRoles {
		roles[i] = string(r)
	}
	execModes := make([]string, len(cs.CoordinationExecutionModes))
	for i, m := range cs.CoordinationExecutionModes {
		execModes[i] = string(m)
	}
	sourceScopes := make([]string, len(cs.SourceScopes))
	for i, s := range cs.SourceScopes {
		sourceScopes[i] = string(s)
	}
	return ports.CapabilitySelectorView{
		ID:                          cs.ID,
		Name:                        cs.Name,
		Kind:                        string(cs.Kind),
		RuntimeFamilies:             runtimeFamilies,
		Tags:                        append([]string{}, cs.Tags...),
		ExcludeTags:                 append([]string{}, cs.ExcludeTags...),
		SourceScopes:                sourceScopes,
		TrustClasses:                trustClasses,
		RiskClasses:                 riskClasses,
		EffectClasses:               effectClasses,
		CoordinationTaskTypes:       append([]string{}, cs.CoordinationTaskTypes...),
		CoordinationRoles:           roles,
		CoordinationExecModes:       execModes,
		CoordinationLongRunning:     int32(cs.CoordinationLongRunning),
		CoordinationDirectInsertion: int32(cs.CoordinationDirectInsertion),
	}
}

func (sp SessionPolicy) ToView() ports.SessionPolicyView {
	sel := sp.Selector
	scopes := make([]string, len(sel.Scopes))
	for i, s := range sel.Scopes {
		scopes[i] = string(s)
	}
	ops := make([]string, len(sel.Operations))
	for i, o := range sel.Operations {
		ops[i] = string(o)
	}
	trustClasses := make([]string, len(sel.TrustClasses))
	for i, tc := range sel.TrustClasses {
		trustClasses[i] = string(tc)
	}
	externalProviders := make([]string, len(sel.ExternalProviders))
	for i, ep := range sel.ExternalProviders {
		externalProviders[i] = string(ep)
	}
	return ports.SessionPolicyView{
		ID:       sp.ID,
		Name:     sp.Name,
		Priority: sp.Priority,
		Enabled:  sp.Enabled,
		Selector: ports.SessionSelectorView{
			Partitions:                append([]string{}, sel.Partitions...),
			ChannelIDs:                append([]string{}, sel.ChannelIDs...),
			Scopes:                    scopes,
			TrustClasses:              trustClasses,
			Operations:                ops,
			ActorKinds:                append([]string{}, sel.ActorKinds...),
			ActorIDs:                  append([]string{}, sel.ActorIDs...),
			ExternalProvider:          externalProviders,
			AuthOnly:                  sel.AuthenticatedOnly,
			RequireOwnership:          sel.RequireOwnership,
			RequireDelegation:         sel.RequireDelegation,
			RequireExternalBinding:    sel.RequireExternalBinding,
			RequireResolvedExternal:   sel.RequireResolvedExternal,
			RequireRestrictedExternal: sel.RequireRestrictedExternal,
		},
		Effect:      string(sp.Effect),
		Approvers:   append([]string{}, sp.Approvers...),
		ApprovalTTL: sp.ApprovalTTL,
		Reason:      sp.Reason,
	}
}
