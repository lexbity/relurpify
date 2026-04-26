package skills

import (
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/manifest"
)

func mergePromptSnippets(base string, snippets []string) string {
	builder := strings.Builder{}
	base = strings.TrimSpace(base)
	if base != "" {
		builder.WriteString(base)
	}
	for _, snippet := range snippets {
		snippet = strings.TrimSpace(snippet)
		if snippet == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(snippet)
	}
	return builder.String()
}

func mergeSkillConfig(base agentspec.AgentSkillConfig, skillSpec manifest.SkillSpec) agentspec.AgentSkillConfig {
	overlay := agentspec.AgentSkillConfig{
		Verification: agentspec.AgentVerificationPolicy{
			SuccessTools:               append([]string{}, skillSpec.Verification.SuccessTools...),
			SuccessCapabilitySelectors: append([]agentspec.SkillCapabilitySelector{}, skillSpec.Verification.SuccessCapabilitySelectors...),
			StopOnSuccess:              skillSpec.Verification.StopOnSuccess,
		},
		Recovery: agentspec.AgentRecoveryPolicy{
			FailureProbeTools:               append([]string{}, skillSpec.Recovery.FailureProbeTools...),
			FailureProbeCapabilitySelectors: append([]agentspec.SkillCapabilitySelector{}, skillSpec.Recovery.FailureProbeCapabilitySelectors...),
		},
		Planning: agentspec.AgentPlanningPolicy{
			RequiredBeforeEdit:          append([]agentspec.SkillCapabilitySelector{}, skillSpec.Planning.RequiredBeforeEdit...),
			PreferredEditCapabilities:   append([]agentspec.SkillCapabilitySelector{}, skillSpec.Planning.PreferredEditCapabilities...),
			PreferredVerifyCapabilities: append([]agentspec.SkillCapabilitySelector{}, skillSpec.Planning.PreferredVerifyCapabilities...),
			StepTemplates:               append([]agentspec.SkillStepTemplate{}, skillSpec.Planning.StepTemplates...),
			RequireVerificationStep:     skillSpec.Planning.RequireVerificationStep,
		},
		Review: agentspec.AgentReviewPolicy{
			Criteria:      append([]string{}, skillSpec.Review.Criteria...),
			FocusTags:     append([]string{}, skillSpec.Review.FocusTags...),
			ApprovalRules: skillSpec.Review.ApprovalRules,
		},
		ContextHints: agentspec.AgentSkillContextHints{
			PreferredDetailLevel: skillSpec.ContextHints.PreferredDetailLevel,
			ProtectPatterns:      append([]string{}, skillSpec.ContextHints.ProtectPatterns...),
		},
	}
	if skillSpec.Review.SeverityWeights != nil {
		overlay.Review.SeverityWeights = make(map[string]float64, len(skillSpec.Review.SeverityWeights))
		for k, v := range skillSpec.Review.SeverityWeights {
			overlay.Review.SeverityWeights[k] = v
		}
	}
	if len(skillSpec.PhaseCapabilities) > 0 {
		overlay.PhaseCapabilities = make(map[string][]string, len(skillSpec.PhaseCapabilities))
		for phase, tools := range skillSpec.PhaseCapabilities {
			overlay.PhaseCapabilities[phase] = append([]string{}, tools...)
		}
	}
	if len(skillSpec.PhaseCapabilitySelectors) > 0 {
		overlay.PhaseCapabilitySelectors = make(map[string][]agentspec.SkillCapabilitySelector, len(skillSpec.PhaseCapabilitySelectors))
		for phase, selectors := range skillSpec.PhaseCapabilitySelectors {
			overlay.PhaseCapabilitySelectors[phase] = append([]agentspec.SkillCapabilitySelector{}, selectors...)
		}
	}
	return agentspec.MergeAgentSpecs(&agentspec.AgentRuntimeSpec{SkillConfig: base}, agentspec.AgentSpecOverlay{SkillConfig: &overlay}).SkillConfig
}
