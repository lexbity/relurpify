package agents

import (
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
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

func mergeSkillConfig(base core.AgentSkillConfig, skillSpec manifest.SkillSpec) core.AgentSkillConfig {
	overlay := core.AgentSkillConfig{
		Verification: core.AgentVerificationPolicy{
			SuccessTools:               append([]string{}, skillSpec.Verification.SuccessTools...),
			SuccessCapabilitySelectors: append([]core.SkillCapabilitySelector{}, skillSpec.Verification.SuccessCapabilitySelectors...),
			StopOnSuccess:              skillSpec.Verification.StopOnSuccess,
		},
		Recovery: core.AgentRecoveryPolicy{
			FailureProbeTools:               append([]string{}, skillSpec.Recovery.FailureProbeTools...),
			FailureProbeCapabilitySelectors: append([]core.SkillCapabilitySelector{}, skillSpec.Recovery.FailureProbeCapabilitySelectors...),
		},
		Planning: core.AgentPlanningPolicy{
			RequiredBeforeEdit:          append([]core.SkillCapabilitySelector{}, skillSpec.Planning.RequiredBeforeEdit...),
			PreferredEditCapabilities:   append([]core.SkillCapabilitySelector{}, skillSpec.Planning.PreferredEditCapabilities...),
			PreferredVerifyCapabilities: append([]core.SkillCapabilitySelector{}, skillSpec.Planning.PreferredVerifyCapabilities...),
			StepTemplates:               append([]core.SkillStepTemplate{}, skillSpec.Planning.StepTemplates...),
			RequireVerificationStep:     skillSpec.Planning.RequireVerificationStep,
		},
		Review: core.AgentReviewPolicy{
			Criteria:      append([]string{}, skillSpec.Review.Criteria...),
			FocusTags:     append([]string{}, skillSpec.Review.FocusTags...),
			ApprovalRules: skillSpec.Review.ApprovalRules,
		},
		ContextHints: core.AgentSkillContextHints{
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
		overlay.PhaseCapabilitySelectors = make(map[string][]core.SkillCapabilitySelector, len(skillSpec.PhaseCapabilitySelectors))
		for phase, selectors := range skillSpec.PhaseCapabilitySelectors {
			overlay.PhaseCapabilitySelectors[phase] = append([]core.SkillCapabilitySelector{}, selectors...)
		}
	}
	return core.MergeAgentSpecs(&core.AgentRuntimeSpec{SkillConfig: base}, core.AgentSpecOverlay{SkillConfig: &overlay}).SkillConfig
}
