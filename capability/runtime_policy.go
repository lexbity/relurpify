package capability

import (
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/safety"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
)

type compiledRuntimePolicy struct {
	agentSpec                  *agentspec.AgentRuntimeSpec
	toolPolicies               map[string]agentspec.ToolPolicy
	capabilityPolicies         []agentspec.CapabilityPolicy
	compiledCapabilityPolicies []compiledCapabilityPolicy
	exposurePolicies           []agentspec.CapabilityExposurePolicy
	compiledExposurePolicies   []compiledExposurePolicy
	globalPolicies             map[string]agentspec.AgentPermissionLevel
	insertionPolicies          []agentspec.CapabilityInsertionPolicy
	providerPolicies           map[string]agentspec.ProviderPolicy
	runtimeSafety              *safety.RuntimeSafetySpec
}

type executionRuntimeState struct {
	agentID   string
	manager   PermissionManagerHandle
	policy    *compiledRuntimePolicy
	safety    *runtimeSafetyController
	telemetry fwtelemetry.Telemetry
}

func compileRuntimePolicy(spec *agentspec.AgentRuntimeSpec, toolPolicies map[string]agentspec.ToolPolicy, capabilityPolicies []agentspec.CapabilityPolicy, exposurePolicies []agentspec.CapabilityExposurePolicy, globalPolicies map[string]agentspec.AgentPermissionLevel) *compiledRuntimePolicy {
	return &compiledRuntimePolicy{
		agentSpec:                  spec,
		toolPolicies:               cloneToolPolicies(toolPolicies),
		capabilityPolicies:         append([]agentspec.CapabilityPolicy{}, capabilityPolicies...),
		compiledCapabilityPolicies: compileCapabilityPolicies(capabilityPolicies),
		exposurePolicies:           append([]agentspec.CapabilityExposurePolicy{}, exposurePolicies...),
		compiledExposurePolicies:   compileExposurePolicies(exposurePolicies),
		globalPolicies:             cloneGlobalPolicies(globalPolicies),
		insertionPolicies:          cloneInsertionPolicies(specInsertionPolicies(spec)),
		providerPolicies:           cloneProviderPolicies(specProviderPolicies(spec)),
		runtimeSafety:              cloneRuntimeSafetySpec(specRuntimeSafety(spec)),
	}
}

func specInsertionPolicies(spec *agentspec.AgentRuntimeSpec) []agentspec.CapabilityInsertionPolicy {
	if spec == nil {
		return nil
	}
	return spec.InsertionPolicies
}

func specProviderPolicies(spec *agentspec.AgentRuntimeSpec) map[string]agentspec.ProviderPolicy {
	if spec == nil {
		return nil
	}
	return spec.ProviderPolicies
}

func specRuntimeSafety(spec *agentspec.AgentRuntimeSpec) *safety.RuntimeSafetySpec {
	if spec == nil {
		return nil
	}
	return spec.RuntimeSafety
}

func (r *CapabilityRegistry) refreshRuntimePolicyLocked() {
	if r == nil {
		return
	}
	r.runtimePolicy = compileRuntimePolicy(r.agentSpec, r.toolPolicies, r.capabilityPolicies, r.exposurePolicies, r.globalPolicies)
}

func (r *CapabilityRegistry) currentRuntimePolicyLocked() *compiledRuntimePolicy {
	if r == nil {
		return &compiledRuntimePolicy{}
	}
	if r.runtimePolicy == nil {
		return &compiledRuntimePolicy{}
	}
	return r.runtimePolicy
}

func (r *CapabilityRegistry) executionRuntimeState() executionRuntimeState {
	if r == nil {
		return executionRuntimeState{policy: &compiledRuntimePolicy{}}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	policy := r.runtimePolicy
	if policy == nil {
		policy = &compiledRuntimePolicy{}
	}
	return executionRuntimeState{
		agentID:   r.registeredAgentID,
		manager:   r.permissionManager,
		policy:    policy,
		safety:    r.safety,
		telemetry: r.telemetry,
	}
}
