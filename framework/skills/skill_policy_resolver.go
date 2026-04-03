package skills

import (
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

type ResolvedSkillPolicy struct {
	PhaseCapabilities               map[string][]string
	VerificationSuccessCapabilities []string
	RecoveryProbeCapabilities       []string
	Planning                        ResolvedPlanningPolicy
	Review                          ResolvedReviewPolicy
}

type EffectiveSkillPolicy struct {
	Spec   *core.AgentRuntimeSpec
	Policy ResolvedSkillPolicy
}

type ResolvedPlanningPolicy struct {
	RequiredBeforeEdit          []string
	PreferredEditCapabilities   []string
	PreferredVerifyCapabilities []string
	StepTemplates               []core.SkillStepTemplate
	RequireVerificationStep     bool
}

type ResolvedReviewPolicy struct {
	Criteria        []string
	FocusTags       []string
	ApprovalRules   core.AgentReviewApprovalRules
	SeverityWeights map[string]float64
}

func ResolveSkillPolicy(registry *capability.Registry, config core.AgentSkillConfig) ResolvedSkillPolicy {
	phaseCapabilities := resolvePhaseCapabilities(registry, config)
	return ResolvedSkillPolicy{
		PhaseCapabilities:               phaseCapabilities,
		VerificationSuccessCapabilities: resolveCapabilityNames(registry, config.Verification.SuccessTools, config.Verification.SuccessCapabilitySelectors),
		RecoveryProbeCapabilities:       resolveCapabilityNames(registry, config.Recovery.FailureProbeTools, config.Recovery.FailureProbeCapabilitySelectors),
		Planning: ResolvedPlanningPolicy{
			RequiredBeforeEdit:          resolveCapabilityNames(registry, nil, config.Planning.RequiredBeforeEdit),
			PreferredEditCapabilities:   resolveCapabilityNames(registry, nil, config.Planning.PreferredEditCapabilities),
			PreferredVerifyCapabilities: resolveCapabilityNames(registry, nil, config.Planning.PreferredVerifyCapabilities),
			StepTemplates:               append([]core.SkillStepTemplate{}, config.Planning.StepTemplates...),
			RequireVerificationStep:     config.Planning.RequireVerificationStep,
		},
		Review: ResolvedReviewPolicy{
			Criteria:        append([]string{}, config.Review.Criteria...),
			FocusTags:       append([]string{}, config.Review.FocusTags...),
			ApprovalRules:   config.Review.ApprovalRules,
			SeverityWeights: cloneSeverityWeights(config.Review.SeverityWeights),
		},
	}
}

func ResolveEffectiveSkillPolicy(task *core.Task, fallback *core.AgentRuntimeSpec, registry *capability.Registry) EffectiveSkillPolicy {
	spec := EffectiveAgentSpec(task, fallback)
	if spec == nil {
		return EffectiveSkillPolicy{}
	}
	return EffectiveSkillPolicy{
		Spec:   spec,
		Policy: ResolveSkillPolicy(registry, spec.SkillConfig),
	}
}

func EffectiveAgentSpec(task *core.Task, fallback *core.AgentRuntimeSpec) *core.AgentRuntimeSpec {
	if task != nil && task.Context != nil {
		if spec, ok := task.Context["agent_spec"].(*core.AgentRuntimeSpec); ok && spec != nil {
			return spec
		}
	}
	return fallback
}

func resolvePhaseCapabilities(registry *capability.Registry, config core.AgentSkillConfig) map[string][]string {
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

func resolveCapabilityNames(registry *capability.Registry, explicit []string, selectors []core.SkillCapabilitySelector) []string {
	var out []string
	for _, name := range explicit {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if registry != nil {
			capability, ok := registry.GetCapability(name)
			if !ok || registry.EffectiveExposure(capability) != core.CapabilityExposureCallable {
				continue
			}
			name = resolvedCapabilityName(capability)
		}
		out = mergeResolvedNames(out, []string{name})
	}
	if len(selectors) == 0 {
		return out
	}
	candidates := registryCapabilitiesSorted(registry)
	for _, selector := range selectors {
		if name := selector.CapabilityName(); name != "" {
			if registry == nil {
				out = mergeResolvedNames(out, []string{name})
				continue
			}
			capability, ok := registry.GetCapability(name)
			if ok && registry.EffectiveExposure(capability) == core.CapabilityExposureCallable && core.SkillSelectorMatchesDescriptor(selector, capability) {
				out = mergeResolvedNames(out, []string{resolvedCapabilityName(capability)})
			}
			continue
		}
		for _, capability := range candidates {
			if core.SkillSelectorMatchesDescriptor(selector, capability) {
				out = mergeResolvedNames(out, []string{resolvedCapabilityName(capability)})
			}
		}
	}
	return out
}

func registryCapabilitiesSorted(registry *capability.Registry) []core.CapabilityDescriptor {
	if registry == nil {
		return nil
	}
	capabilities := registry.CallableCapabilities()
	sort.Slice(capabilities, func(i, j int) bool {
		return resolvedCapabilityName(capabilities[i]) < resolvedCapabilityName(capabilities[j])
	})
	return capabilities
}

func resolvedCapabilityName(capability core.CapabilityDescriptor) string {
	if name := strings.TrimSpace(capability.Name); name != "" {
		return name
	}
	return strings.TrimSpace(capability.ID)
}

func cloneResolvedPhaseCapabilities(input map[string][]string) map[string][]string {
	if input == nil {
		return nil
	}
	out := make(map[string][]string, len(input))
	for phase, values := range input {
		out[phase] = append([]string{}, values...)
	}
	return out
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
