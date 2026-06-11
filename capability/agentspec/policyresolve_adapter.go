package agentspec

import (
	"math"

	"codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/governance/policyresolve"
)

// ToPolicyResolveOrchConfig converts an AgentOrchestrationConfig to
// policyresolve.AgentOrchestrationConfig so callers can pass orchestration
// configuration to policyresolve without importing policyresolve.
func ToPolicyResolveOrchConfig(cfg AgentOrchestrationConfig) policyresolve.AgentOrchestrationConfig {
	return policyresolve.AgentOrchestrationConfig{
		PhaseCapabilities:        cloneMap(cfg.PhaseCapabilities),
		PhaseCapabilitySelectors: toCapSelectorsMap(cfg.PhaseCapabilitySelectors),
		Verification:             toVerificationPolicy(cfg.Verification),
		Recovery:                 toRecoveryPolicy(cfg.Recovery),
		Planning:                 toPlanningPolicy(cfg.Planning),
		Review:                   toReviewPolicy(cfg.Review),
	}
}

func toCapSelectorsMap(in map[string][]CapabilitySelector) map[string][]policyresolve.CapabilitySelector {
	if in == nil {
		return nil
	}
	out := make(map[string][]policyresolve.CapabilitySelector, len(in))
	for k, selectors := range in {
		out[k] = toCapSelectors(selectors)
	}
	return out
}

func toCapSelectors(in []CapabilitySelector) []policyresolve.CapabilitySelector {
	if in == nil {
		return nil
	}
	out := make([]policyresolve.CapabilitySelector, len(in))
	for i, s := range in {
		out[i] = policyresolve.CapabilitySelector{
			ID:                          s.ID,
			Name:                        s.Name,
			Kind:                        string(s.Kind),
			RuntimeFamilies:             toStringSlice(s.RuntimeFamilies),
			Tags:                        append([]string(nil), s.Tags...),
			ExcludeTags:                 append([]string(nil), s.ExcludeTags...),
			SourceScopes:                toStringSlice(s.SourceScopes),
			TrustClasses:                toStringSlice(s.TrustClasses),
			RiskClasses:                 toStringSlice(s.RiskClasses),
			EffectClasses:               toStringSlice(s.EffectClasses),
			CoordinationRoles:           toStringSlice(s.CoordinationRoles),
			CoordinationTaskTypes:       append([]string(nil), s.CoordinationTaskTypes...),
			CoordinationExecutionModes:  toStringSlice(s.CoordinationExecutionModes),
			CoordinationLongRunning:     clampSpec(int(s.CoordinationLongRunning)),
			CoordinationDirectInsertion: clampSpec(int(s.CoordinationDirectInsertion)),
		}
	}
	return out
}

func toVerificationPolicy(v AgentVerificationPolicy) policyresolve.AgentVerificationPolicy {
	return policyresolve.AgentVerificationPolicy{
		SuccessTools:               append([]string(nil), v.SuccessTools...),
		SuccessCapabilitySelectors: toCapSelectors(v.SuccessCapabilitySelectors),
		StopOnSuccess:              v.StopOnSuccess,
	}
}

func toRecoveryPolicy(r AgentRecoveryPolicy) policyresolve.AgentRecoveryPolicy {
	return policyresolve.AgentRecoveryPolicy{
		FailureProbeTools:               append([]string(nil), r.FailureProbeTools...),
		FailureProbeCapabilitySelectors: toCapSelectors(r.FailureProbeCapabilitySelectors),
	}
}

func toPlanningPolicy(p AgentPlanningPolicy) policyresolve.AgentPlanningPolicy {
	return policyresolve.AgentPlanningPolicy{
		RequiredBeforeEdit:          toCapSelectors(p.RequiredBeforeEdit),
		PreferredEditCapabilities:   toCapSelectors(p.PreferredEditCapabilities),
		PreferredVerifyCapabilities: toCapSelectors(p.PreferredVerifyCapabilities),
		StepTemplates:               toSkillTemplates(p.StepTemplates),
		RequireVerificationStep:     p.RequireVerificationStep,
	}
}

func toSkillTemplates(in []SkillStepTemplate) []policy.SkillStepTemplate {
	if in == nil {
		return nil
	}
	out := make([]policy.SkillStepTemplate, len(in))
	for i, s := range in {
		out[i] = policy.SkillStepTemplate{Kind: s.Kind, Description: s.Description}
	}
	return out
}

func toReviewPolicy(r AgentReviewPolicy) policyresolve.AgentReviewPolicy {
	return policyresolve.AgentReviewPolicy{
		Criteria:  append([]string(nil), r.Criteria...),
		FocusTags: append([]string(nil), r.FocusTags...),
		ApprovalRules: policy.AgentReviewApprovalRules{
			RequireVerificationEvidence: r.ApprovalRules.RequireVerificationEvidence,
			RejectOnUnresolvedErrors:    r.ApprovalRules.RejectOnUnresolvedErrors,
		},
		SeverityWeights: cloneMapF64(r.SeverityWeights),
	}
}

func clampSpec(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}

func toStringSlice[T ~string](in []T) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = string(v)
	}
	return out
}

func cloneMap[V any](in map[string]V) map[string]V {
	if in == nil {
		return nil
	}
	out := make(map[string]V, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneMapF64(in map[string]float64) map[string]float64 {
	if in == nil {
		return nil
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
