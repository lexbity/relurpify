package policyresolve

import (
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/governance/classification"
	"codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/governance/risk"
)

// RegistryView is the governance-owned view of the capability registry for
// policy resolution. Descriptors are passed as any; callers must ensure
// values returned by GetCapability and CallableCapabilities satisfy
// ports.DescriptorView.
type RegistryView interface {
	GetCapability(idOrName string) (any, bool)
	CallableCapabilities() []any
	EffectiveExposure(desc any) string
}

// ResolvedCapabilitySelector is a local type matching agentspec.CapabilitySelector fields
// needed for policy resolution.
type ResolvedCapabilitySelector struct {
	ID                          string
	Name                        string
	Kind                        string
	RuntimeFamilies             []string
	Tags                        []string
	ExcludeTags                 []string
	SourceScopes                []string
	TrustClasses                []string
	RiskClasses                 []string
	EffectClasses               []string
	CoordinationRoles           []string
	CoordinationTaskTypes       []string
	CoordinationExecutionModes  []string
	CoordinationLongRunning     int32
	CoordinationDirectInsertion int32
}

// ResolvedAgentVerificationPolicy holds verification-related orchestration config.
type ResolvedAgentVerificationPolicy struct {
	SuccessTools               []string
	SuccessCapabilitySelectors []ResolvedCapabilitySelector
	StopOnSuccess              bool
}

// ResolvedAgentRecoveryPolicy holds recovery-related orchestration config.
type ResolvedAgentRecoveryPolicy struct {
	FailureProbeTools               []string
	FailureProbeCapabilitySelectors []ResolvedCapabilitySelector
}

// ResolvedAgentPlanningPolicy holds planning-related orchestration config.
type ResolvedAgentPlanningPolicy struct {
	RequiredBeforeEdit          []ResolvedCapabilitySelector
	PreferredEditCapabilities   []ResolvedCapabilitySelector
	PreferredVerifyCapabilities []ResolvedCapabilitySelector
	StepTemplates               []policy.ResolvedSkillStepTemplate
	RequireVerificationStep     bool
}

// ResolvedAgentReviewPolicy holds review-related orchestration config.
type ResolvedAgentReviewPolicy struct {
	Criteria        []string
	FocusTags       []string
	ApprovalRules   policy.ResolvedAgentReviewApprovalRules
	SeverityWeights map[string]float64
}

// ResolvedAgentOrchestrationConfig mirrors agentspec.AgentOrchestrationConfig fields
// needed for policy resolution.
type ResolvedAgentOrchestrationConfig struct {
	PhaseCapabilities        map[string][]string
	PhaseCapabilitySelectors map[string][]ResolvedCapabilitySelector
	Verification             ResolvedAgentVerificationPolicy
	Recovery                 ResolvedAgentRecoveryPolicy
	Planning                 ResolvedAgentPlanningPolicy
	Review                   ResolvedAgentReviewPolicy
}

var (
	capabilityExposureCallable = "callable" //nolint:gochecknoglobals // immutable policy vocabulary constant
)

// ResolveAgentPolicy resolves the agent spec's orchestration configuration
// against the capability registry to produce a policy.ResolvedAgentPolicy.
func ResolveAgentPolicy(registry RegistryView, config ResolvedAgentOrchestrationConfig) policy.ResolvedAgentPolicy {
	phaseCapabilities := resolvePhaseCapabilities(registry, config)
	return policy.ResolvedAgentPolicy{
		PhaseCapabilities:               phaseCapabilities,
		VerificationSuccessCapabilities: resolveCapabilityNames(registry, config.Verification.SuccessTools, config.Verification.SuccessCapabilitySelectors),
		RecoveryProbeCapabilities:       resolveCapabilityNames(registry, config.Recovery.FailureProbeTools, config.Recovery.FailureProbeCapabilitySelectors),
		Planning: policy.ResolvedPlanningPolicy{
			RequiredBeforeEdit:          resolveCapabilityNames(registry, nil, config.Planning.RequiredBeforeEdit),
			PreferredEditCapabilities:   resolveCapabilityNames(registry, nil, config.Planning.PreferredEditCapabilities),
			PreferredVerifyCapabilities: resolveCapabilityNames(registry, nil, config.Planning.PreferredVerifyCapabilities),
			StepTemplates:               config.Planning.StepTemplates,
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

// ResolveEffectiveAgentPolicy resolves policy from the effective orchestration config.
func ResolveEffectiveAgentPolicy(cfg ResolvedAgentOrchestrationConfig, registry RegistryView) policy.EffectiveAgentPolicy {
	return policy.EffectiveAgentPolicy{
		Spec:   cfg,
		Policy: ResolveAgentPolicy(registry, cfg),
	}
}

func resolvePhaseCapabilities(registry RegistryView, config ResolvedAgentOrchestrationConfig) map[string][]string {
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

func resolveCapabilityNames(registry RegistryView, explicit []string, selectors []ResolvedCapabilitySelector) []string {
	var out []string
	for _, name := range explicit {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if registry != nil {
			cd, ok := registry.GetCapability(name)
			if !ok || registry.EffectiveExposure(cd) != capabilityExposureCallable {
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
			if ok && registry.EffectiveExposure(cd) == capabilityExposureCallable && selectorMatchesDescriptor(selector, cd) {
				out = mergeResolvedNames(out, []string{resolvedCapabilityName(cd)})
			}
			continue
		}
		for _, cd := range candidates {
			if selectorMatchesDescriptor(selector, cd) {
				out = mergeResolvedNames(out, []string{resolvedCapabilityName(cd)})
			}
		}
	}
	return out
}

func selectorCapabilityName(selector ResolvedCapabilitySelector) string {
	if name := strings.TrimSpace(selector.Name); name != "" {
		return name
	}
	return strings.TrimSpace(selector.ID)
}

func registryCapabilitiesSorted(registry RegistryView) []any {
	if registry == nil {
		return nil
	}
	capabilities := registry.CallableCapabilities()
	sort.Slice(capabilities, func(i, j int) bool {
		return resolvedCapabilityName(capabilities[i]) < resolvedCapabilityName(capabilities[j])
	})
	return capabilities
}

func resolvedCapabilityName(desc any) string {
	d, ok := desc.(ports.DescriptorView)
	if !ok {
		return ""
	}
	if name := strings.TrimSpace(d.CapabilityName()); name != "" {
		return name
	}
	return strings.TrimSpace(d.CapabilityID())
}

func selectorMatchesDescriptor(selector ResolvedCapabilitySelector, desc any) bool {
	d, ok := desc.(ports.DescriptorView)
	if !ok {
		return false
	}
	if strings.TrimSpace(selector.ID) != "" && !strings.EqualFold(strings.TrimSpace(selector.ID), d.CapabilityID()) {
		return false
	}
	if strings.TrimSpace(selector.Name) != "" && !strings.EqualFold(strings.TrimSpace(selector.Name), d.CapabilityName()) {
		return false
	}
	if selector.Kind != "" && selector.Kind != d.CapabilityKind() {
		return false
	}
	if len(selector.RuntimeFamilies) > 0 && !containsAny(selector.RuntimeFamilies, d.RuntimeFamily()) {
		return false
	}
	if len(selector.Tags) > 0 && !containsAll(selector.Tags, d.Tags()) {
		return false
	}
	if len(selector.ExcludeTags) > 0 && containsAnyInSlice(selector.ExcludeTags, d.Tags()) {
		return false
	}
	if len(selector.SourceScopes) > 0 && !containsAny(selector.SourceScopes, d.SourceScope()) {
		return false
	}
	if len(selector.TrustClasses) > 0 && !containsAny(selector.TrustClasses, d.TrustClass()) {
		return false
	}
	if len(selector.RiskClasses) > 0 && !containsAnyInRiskClass(selector.RiskClasses, d.RiskClasses()) {
		return false
	}
	if len(selector.EffectClasses) > 0 && !containsAnyInEffectClass(selector.EffectClasses, d.EffectClasses()) {
		return false
	}
	if len(selector.CoordinationRoles) > 0 && !containsAny(selector.CoordinationRoles, d.CoordinationRole()) {
		return false
	}
	if len(selector.CoordinationTaskTypes) > 0 && !containsAll(selector.CoordinationTaskTypes, d.CoordinationTaskTypes()) {
		return false
	}
	if len(selector.CoordinationExecutionModes) > 0 && !containsAnyInSlice(selector.CoordinationExecutionModes, d.CoordinationExecutionModes()) {
		return false
	}
	if selector.CoordinationLongRunning != 0 {
		if d.CoordinationLongRunning() != selector.CoordinationLongRunning {
			return false
		}
	}
	if selector.CoordinationDirectInsertion != 0 {
		if d.CoordinationDirectInsertionAllowed() != selector.CoordinationDirectInsertion {
			return false
		}
	}
	return true
}

func containsAny(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func containsAnyInSlice(values, wants []string) bool {
	for _, w := range wants {
		for _, v := range values {
			if v == w {
				return true
			}
		}
	}
	return false
}

func containsAnyInRiskClass(values []string, want []risk.RiskClass) bool {
	for _, w := range want {
		for _, v := range values {
			if string(w) == v {
				return true
			}
		}
	}
	return false
}

func containsAnyInEffectClass(values []string, want []classification.EffectClass) bool {
	for _, w := range want {
		for _, v := range values {
			if string(w) == v {
				return true
			}
		}
	}
	return false
}

func containsAll(values []string, haystack []string) bool {
	for _, want := range values {
		if !containsAny(haystack, want) {
			return false
		}
	}
	return true
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
