package policyresolve

import (
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	capability "codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/governance/policy"
)

// ResolveAgentPolicy resolves the agent spec's orchestration configuration
// against the capability registry to produce a policy.ResolvedAgentPolicy.
func ResolveAgentPolicy(registry *capability.Registry, config agentspec.AgentOrchestrationConfig) policy.ResolvedAgentPolicy {
	phaseCapabilities := resolvePhaseCapabilities(registry, config)
	return policy.ResolvedAgentPolicy{
		PhaseCapabilities:               phaseCapabilities,
		VerificationSuccessCapabilities: resolveCapabilityNames(registry, config.Verification.SuccessTools, config.Verification.SuccessCapabilitySelectors),
		RecoveryProbeCapabilities:       resolveCapabilityNames(registry, config.Recovery.FailureProbeTools, config.Recovery.FailureProbeCapabilitySelectors),
		Planning: policy.ResolvedPlanningPolicy{
			RequiredBeforeEdit:          resolveCapabilityNames(registry, nil, config.Planning.RequiredBeforeEdit),
			PreferredEditCapabilities:   resolveCapabilityNames(registry, nil, config.Planning.PreferredEditCapabilities),
			PreferredVerifyCapabilities: resolveCapabilityNames(registry, nil, config.Planning.PreferredVerifyCapabilities),
			StepTemplates:               append([]agentspec.SkillStepTemplate{}, config.Planning.StepTemplates...),
			RequireVerificationStep:     config.Planning.RequireVerificationStep,
		},
		Review: policy.ResolvedReviewPolicy{
			Criteria:        append([]string{}, config.Review.Criteria...),
			FocusTags:       append([]string{}, config.Review.FocusTags...),
			ApprovalRules:   config.Review.ApprovalRules,
			SeverityWeights: cloneSeverityWeights(config.Review.SeverityWeights),
		},
	}
}

// ResolveEffectiveAgentPolicy resolves policy from the effective spec,
// falling back through task context and the provided fallback spec.
func ResolveEffectiveAgentPolicy(task *capability.Task, fallback *agentspec.AgentRuntimeSpec, registry *capability.Registry) policy.EffectiveAgentPolicy {
	spec := effectiveSpec(task, fallback)
	if spec == nil {
		return policy.EffectiveAgentPolicy{}
	}
	return policy.EffectiveAgentPolicy{
		Spec:   spec,
		Policy: ResolveAgentPolicy(registry, spec.Orchestration),
	}
}

func effectiveSpec(task *capability.Task, fallback *agentspec.AgentRuntimeSpec) *agentspec.AgentRuntimeSpec {
	if task != nil && task.Context != nil {
		if spec, ok := task.Context["agent_spec"].(*agentspec.AgentRuntimeSpec); ok && spec != nil {
			return spec
		}
	}
	return fallback
}

func resolvePhaseCapabilities(registry *capability.Registry, config agentspec.AgentOrchestrationConfig) map[string][]string {
	if len(config.PhaseCapabilities) == 0 && len(config.PhaseCapabilitySelectors) == 0 {
		return nil
	}
	out := make(map[string][]string)
	for phase, capabilities := range config.PhaseCapabilities {
		out[phase] = mergeResolvedNames(out[phase], resolveCapabilityNames(registry, capabilities, nil))
	}
	for phase, selectors := range config.PhaseCapabilitySelectors {
		out[phase] = mergeResolvedNames(out[phase], resolveCapabilityNames(registry, nil, selectors))
	}
	return out
}

func resolveCapabilityNames(registry *capability.Registry, explicit []string, selectors []agentspec.CapabilitySelector) []string {
	var out []string
	for _, name := range explicit {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if registry != nil {
			cd, ok := registry.GetCapability(name)
			if !ok || registry.EffectiveExposure(cd) != capability.CapabilityExposureCallable {
				continue
			}
			name = resolvedCapabilityName(cd)
		}
		out = mergeResolvedNames(out, []string{name})
	}
	if len(selectors) == 0 {
		return out
	}
	candidates := registryCapabilitiesSorted(registry)
	for _, selector := range selectors {
		if name := selectorCapabilityName(selector); name != "" {
			if registry == nil {
				out = mergeResolvedNames(out, []string{name})
				continue
			}
			cd, ok := registry.GetCapability(name)
			if ok && registry.EffectiveExposure(cd) == capability.CapabilityExposureCallable && capability.SelectorMatchesDescriptor(selector, cd) {
				out = mergeResolvedNames(out, []string{resolvedCapabilityName(cd)})
			}
			continue
		}
		for _, cd := range candidates {
			if capability.SelectorMatchesDescriptor(selector, cd) {
				out = mergeResolvedNames(out, []string{resolvedCapabilityName(cd)})
			}
		}
	}
	return out
}

func selectorCapabilityName(selector agentspec.CapabilitySelector) string {
	if name := strings.TrimSpace(selector.Name); name != "" {
		return name
	}
	return strings.TrimSpace(selector.ID)
}

func registryCapabilitiesSorted(registry *capability.Registry) []capability.CapabilityDescriptor {
	if registry == nil {
		return nil
	}
	capabilities := registry.CallableCapabilities()
	sort.Slice(capabilities, func(i, j int) bool {
		return resolvedCapabilityName(capabilities[i]) < resolvedCapabilityName(capabilities[j])
	})
	return capabilities
}

func resolvedCapabilityName(cap capability.CapabilityDescriptor) string {
	if name := strings.TrimSpace(cap.Name); name != "" {
		return name
	}
	return strings.TrimSpace(cap.ID)
}

func mergeResolvedNames(base, extra []string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, name := range append(append([]string{}, base...), extra...) {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func cloneSeverityWeights(input map[string]float64) map[string]float64 {
	if input == nil {
		return nil
	}
	out := make(map[string]float64, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
