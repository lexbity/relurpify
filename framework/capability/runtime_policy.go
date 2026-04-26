package capability

import (
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/core"
)

type compiledRuntimePolicy struct {
	agentSpec                  *AgentRuntimeSpec
	toolPolicies               map[string]ToolPolicy
	capabilityPolicies         []core.CapabilityPolicy
	compiledCapabilityPolicies []compiledCapabilityPolicy
	exposurePolicies           []core.CapabilityExposurePolicy
	compiledExposurePolicies   []compiledExposurePolicy
	globalPolicies             map[string]AgentPermissionLevel
	insertionPolicies          []core.CapabilityInsertionPolicy
	providerPolicies           map[string]core.ProviderPolicy
	runtimeSafety              *agentspec.RuntimeSafetySpec
}

type executionRuntimeState struct {
	agentID   string
	manager   *PermissionManager
	policy    *compiledRuntimePolicy
	safety    *runtimeSafetyController
	telemetry Telemetry
}

func compileRuntimePolicy(spec *AgentRuntimeSpec, toolPolicies map[string]ToolPolicy, capabilityPolicies []core.CapabilityPolicy, exposurePolicies []core.CapabilityExposurePolicy, globalPolicies map[string]AgentPermissionLevel) *compiledRuntimePolicy {
	return &compiledRuntimePolicy{
		agentSpec:                  spec,
		toolPolicies:               cloneToolPolicies(toolPolicies),
		capabilityPolicies:         append([]core.CapabilityPolicy{}, capabilityPolicies...),
		compiledCapabilityPolicies: compileCapabilityPolicies(capabilityPolicies),
		exposurePolicies:           append([]core.CapabilityExposurePolicy{}, exposurePolicies...),
		compiledExposurePolicies:   compileExposurePolicies(exposurePolicies),
		globalPolicies:             cloneGlobalPolicies(globalPolicies),
		insertionPolicies:          cloneInsertionPolicies(specInsertionPolicies(spec)),
		providerPolicies:           cloneProviderPolicies(specProviderPolicies(spec)),
		runtimeSafety:              cloneRuntimeSafetySpec(specRuntimeSafety(spec)),
	}
}

func specInsertionPolicies(spec *AgentRuntimeSpec) []core.CapabilityInsertionPolicy {
	if spec == nil {
		return nil
	}
	return spec.InsertionPolicies
}

func specProviderPolicies(spec *AgentRuntimeSpec) map[string]core.ProviderPolicy {
	if spec == nil {
		return nil
	}
	return spec.ProviderPolicies
}

func specRuntimeSafety(spec *AgentRuntimeSpec) *agentspec.RuntimeSafetySpec {
	if spec == nil {
		return nil
	}
	if spec.RuntimeSafety == nil {
		return nil
	}
	return core.RuntimeSafetySpecFromAgentSpec(spec.RuntimeSafety)
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
