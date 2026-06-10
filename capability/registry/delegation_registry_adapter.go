package registry

import (
	"context"

	capresult "codeburg.org/lexbit/relurpify/capability/result"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	agentspec "codeburg.org/lexbit/relurpify/capability/agentspec"
	ports "codeburg.org/lexbit/relurpify/capability/ports"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/capability/classification"
	"codeburg.org/lexbit/relurpify/governance/risk"
)

type DelegationRegistry struct {
	inner *CapabilityRegistry
}

func NewDelegationRegistry(inner *CapabilityRegistry) *DelegationRegistry {
	return &DelegationRegistry{inner: inner}
}

func (r *DelegationRegistry) GetCoordinationTarget(idOrName string) (governanceports.DescriptorView, bool) {
	if r.inner == nil {
		return nil, false
	}
	dt, ok := r.inner.GetCoordinationTarget(idOrName)
	if !ok {
		return nil, false
	}
	desc, ok := dt.(descriptor.CapabilityDescriptor)
	if !ok {
		return nil, false
	}
	return descriptor.CapabilityDescriptorView(desc), true
}

func (r *DelegationRegistry) CoordinationTargets(selectors ...governanceports.CapabilitySelectorView) []governanceports.DescriptorView {
	if r.inner == nil {
		return nil
	}
	sels := make([]agentspec.CapabilitySelector, len(selectors))
	for i, s := range selectors {
		sels[i] = capabilitySelectorFromView(s)
	}
	targets := r.inner.CoordinationTargets(sels...)
	out := make([]governanceports.DescriptorView, len(targets))
	for i, t := range targets {
		if cd, ok := t.(descriptor.CapabilityDescriptor); ok {
			out[i] = descriptor.CapabilityDescriptorView(cd)
		}
	}
	return out
}

func (r *DelegationRegistry) InvokeCapability(ctx context.Context, state ports.State, idOrName string, args map[string]interface{}) (any, error) {
	if r.inner == nil {
		return nil, nil
	}
	return r.inner.InvokeCapability(ctx, state, idOrName, args)
}

func (r *DelegationRegistry) CapturePolicySnapshot() *policy.PolicySnapshot {
	if r.inner == nil {
		return nil
	}
	snap := r.inner.CapturePolicySnapshot()
	if snap == nil {
		return nil
	}
	return &policy.PolicySnapshot{ID: snap.ID}
}

func (r *DelegationRegistry) EffectiveCoordination(spec governanceports.SpecView) governanceports.CoordinationSpecView {
	agentSpec, _ := spec.(*agentspec.AgentRuntimeSpec)
	coord := agentspec.EffectiveCoordination(agentSpec)
	return governanceports.CoordinationSpecView{
		MaxDelegationDepth:        coord.MaxDelegationDepth,
		AllowRemoteDelegation:     coord.AllowRemoteDelegation,
		AllowBackgroundDelegation: coord.AllowBackgroundDelegation,
		RequireApprovalCrossTrust: coord.RequireApprovalCrossTrust,
		DelegationTargetSelectors: selectorsToViews(coord.DelegationTargetSelectors),
	}
}

func (r *DelegationRegistry) BuildDelegationResult(request policy.DelegationRequest, target governanceports.DescriptorView, result any, invokeErr error, snapshot *policy.PolicySnapshot, spec governanceports.SpecView, callerTrust string) *policy.DelegationResult {
	desc := descriptorFromView(target)
	toolResult, _ := result.(*ports.ToolResult)
	agentSpec, _ := spec.(*agentspec.AgentRuntimeSpec)

	var capSnap *capresult.PolicySnapshot
	if snapshot != nil {
		capSnap = &capresult.PolicySnapshot{ID: snapshot.ID}
	}

	if invokeErr != nil {
		failed := policy.NewDelegationResult(request, desc.ID, desc.Source.ProviderID, desc.Source.SessionID, policy.DelegationStateFailed, false, nil)
		failed.Diagnostics = []string{invokeErr.Error()}
		return failed
	}
	if toolResult == nil {
		toolResult = &ports.ToolResult{Success: true}
	}
	state := policy.DelegationStateSucceeded
	if !toolResult.Success {
		state = policy.DelegationStateFailed
	}
	delegationResult := policy.NewDelegationResult(request, desc.ID, desc.Source.ProviderID, desc.Source.SessionID, state, toolResult.Success, toolResult.Data)
	if toolResult.Error != "" {
		delegationResult.Diagnostics = append(delegationResult.Diagnostics, toolResult.Error)
	}

	var approval *capresult.ApprovalBinding
	if pb := policy.ApprovalBindingFromDelegation(request, delegationResult); pb != nil {
		approval = &capresult.ApprovalBinding{CapabilityID: pb.CapabilityID, TaskID: pb.TaskID, WorkflowID: pb.WorkflowID}
	}

	envelope := capresult.NewCapabilityResultEnvelope(desc, toolResult, capresult.ContentDispositionRaw, capSnap, approval)
	decision := EffectiveInsertionDecision(agentSpec, envelope)

	if desc.Coordination != nil && desc.Coordination.DirectInsertionAllowed != descriptor.EnabledStateEnabled && decision.Action == agentspec.InsertionActionDirect {
		decision.Action = agentspec.InsertionActionSummarized
		decision.Reason = "coordination target requires summarized insertion"
	}
	if callerTrust != "" && callerTrust != string(desc.TrustClass) {
		coord := agentspec.EffectiveCoordination(agentSpec)
		if coord.RequireApprovalCrossTrust {
			decision.Action = agentspec.InsertionActionHITLRequired
			decision.RequiresHITL = true
			decision.Reason = "cross-trust delegation requires approval"
		}
	}

	delegationResult.Provenance = envelope.Provenance
	delegationResult.Disposition = envelope.Disposition
	policy.ApplyDelegationInsertionDecision(delegationResult, decision)
	return delegationResult
}

func descriptorFromView(view governanceports.DescriptorView) descriptor.CapabilityDescriptor {
	if adapter, ok := view.(*descriptor.DescriptorViewAdapter); ok {
		return adapter.D
	}
	return descriptor.CapabilityDescriptor{
		ID:            view.CapabilityID(),
		Name:          view.CapabilityName(),
		TrustClass:    agentspec.TrustClass(view.TrustClass()),
		RuntimeFamily: agentspec.CapabilityRuntimeFamily(view.RuntimeFamily()),
		Source: descriptor.CapabilitySource{
			ProviderID: view.SourceProviderID(),
			Scope:      classification.CapabilityScope(view.SourceScope()),
			SessionID:  view.SourceSessionID(),
		},
	}
}

func selectorsToViews(selectors []agentspec.CapabilitySelector) []governanceports.CapabilitySelectorView {
	out := make([]governanceports.CapabilitySelectorView, len(selectors))
	for i, s := range selectors {
		out[i] = s.ToView()
	}
	return out
}

func capabilitySelectorFromView(v governanceports.CapabilitySelectorView) agentspec.CapabilitySelector {
	return agentspec.CapabilitySelector{
		ID:                          v.ID,
		Name:                        v.Name,
		Kind:                        agentspec.CapabilityKind(v.Kind),
		RuntimeFamilies:             stringSliceToRuntimeFamilies(v.RuntimeFamilies),
		Tags:                        copyStrings(v.Tags),
		ExcludeTags:                 copyStrings(v.ExcludeTags),
		SourceScopes:                stringSliceToScopes(v.SourceScopes),
		TrustClasses:                stringSliceToTrustClasses(v.TrustClasses),
		RiskClasses:                 stringSliceToRiskClasses(v.RiskClasses),
		EffectClasses:               stringSliceToEffectClasses(v.EffectClasses),
		CoordinationRoles:           stringSliceToCoordinationRoles(v.CoordinationRoles),
		CoordinationTaskTypes:       copyStrings(v.CoordinationTaskTypes),
		CoordinationExecutionModes:  stringSliceToExecutionModes(v.CoordinationExecModes),
		CoordinationLongRunning:     agentspec.EnabledState(v.CoordinationLongRunning),
		CoordinationDirectInsertion: agentspec.EnabledState(v.CoordinationDirectInsertion),
	}
}

func copyStrings(s []string) []string {
	if s == nil {
		return nil
	}
	out := make([]string, len(s))
	copy(out, s)
	return out
}

func stringSliceToRuntimeFamilies(s []string) []agentspec.CapabilityRuntimeFamily {
	out := make([]agentspec.CapabilityRuntimeFamily, len(s))
	for i, v := range s {
		out[i] = agentspec.CapabilityRuntimeFamily(v)
	}
	return out
}

func stringSliceToScopes(s []string) []classification.CapabilityScope {
	out := make([]classification.CapabilityScope, len(s))
	for i, v := range s {
		out[i] = classification.CapabilityScope(v)
	}
	return out
}

func stringSliceToTrustClasses(s []string) []agentspec.TrustClass {
	out := make([]agentspec.TrustClass, len(s))
	for i, v := range s {
		out[i] = agentspec.TrustClass(v)
	}
	return out
}

func stringSliceToRiskClasses(s []string) []risk.RiskClass {
	out := make([]risk.RiskClass, len(s))
	for i, v := range s {
		out[i] = risk.RiskClass(v)
	}
	return out
}

func stringSliceToEffectClasses(s []string) []classification.EffectClass {
	out := make([]classification.EffectClass, len(s))
	for i, v := range s {
		out[i] = classification.EffectClass(v)
	}
	return out
}

func stringSliceToCoordinationRoles(s []string) []agentspec.CoordinationRole {
	out := make([]agentspec.CoordinationRole, len(s))
	for i, v := range s {
		out[i] = agentspec.CoordinationRole(v)
	}
	return out
}

func stringSliceToExecutionModes(s []string) []agentspec.CoordinationExecutionMode {
	out := make([]agentspec.CoordinationExecutionMode, len(s))
	for i, v := range s {
		out[i] = agentspec.CoordinationExecutionMode(v)
	}
	return out
}
