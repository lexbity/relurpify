package policy

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
)

// ResolvedAgentPolicy carries the resolved agent policy collapsed from the
// agent spec's orchestration configuration.  It is what the agent strategies
// (planner, react, reflection) read at runtime.
type ResolvedAgentPolicy struct {
	PhaseCapabilities               map[string][]string
	VerificationSuccessCapabilities []string
	RecoveryProbeCapabilities       []string
	Planning                        ResolvedPlanningPolicy
	Review                          ResolvedReviewPolicy
}

// EffectiveAgentPolicy bundles the spec with its resolved 
type EffectiveAgentPolicy struct {
	Spec   *agentspec.AgentRuntimeSpec
	Policy ResolvedAgentPolicy
}

// ResolvedPlanningPolicy carries plan-step 
type ResolvedPlanningPolicy struct {
	RequiredBeforeEdit          []string
	PreferredEditCapabilities   []string
	PreferredVerifyCapabilities []string
	StepTemplates               []agentspec.SkillStepTemplate
	RequireVerificationStep     bool
}

// ResolvedReviewPolicy carries review-phase 
type ResolvedReviewPolicy struct {
	Criteria        []string
	FocusTags       []string
	ApprovalRules   agentspec.AgentReviewApprovalRules
	SeverityWeights map[string]float64
}

// PlanningRenderOptions controls what RenderPlanningPolicy includes.
type PlanningRenderOptions struct {
	IncludePhaseCapabilities   bool
	IncludeVerificationSuccess bool
	VerificationRequirement    string
}

// RenderPlanningPolicy renders planning constraints into a prompt string.
func RenderPlanningPolicy(policy ResolvedAgentPolicy, options PlanningRenderOptions) string {
	var lines []string
	if options.IncludePhaseCapabilities {
		if capabilities := policy.PhaseCapabilities["explore"]; len(capabilities) > 0 {
			lines = append(lines, "Explore capabilities: "+strings.Join(capabilities, ", "))
		}
		if capabilities := policy.PhaseCapabilities["edit"]; len(capabilities) > 0 {
			lines = append(lines, "Edit capabilities: "+strings.Join(capabilities, ", "))
		}
		if capabilities := policy.PhaseCapabilities["verify"]; len(capabilities) > 0 {
			lines = append(lines, "Verify capabilities: "+strings.Join(capabilities, ", "))
		}
	}
	if options.IncludeVerificationSuccess {
		if capabilities := policy.VerificationSuccessCapabilities; len(capabilities) > 0 {
			lines = append(lines, "Verification success capabilities: "+strings.Join(capabilities, ", "))
		}
	}
	if capabilities := policy.Planning.RequiredBeforeEdit; len(capabilities) > 0 {
		lines = append(lines, "policy.Required before edit: "+strings.Join(capabilities, ", "))
	}
	if capabilities := policy.Planning.PreferredEditCapabilities; len(capabilities) > 0 {
		lines = append(lines, "Preferred edit capabilities: "+strings.Join(capabilities, ", "))
	}
	if capabilities := policy.Planning.PreferredVerifyCapabilities; len(capabilities) > 0 {
		lines = append(lines, "Preferred verify capabilities: "+strings.Join(capabilities, ", "))
	}
	if steps := policy.Planning.StepTemplates; len(steps) > 0 {
		var rendered []string
		for _, step := range steps {
			rendered = append(rendered, fmt.Sprintf("%s: %s", step.Kind, step.Description))
		}
		lines = append(lines, "Preferred step templates: "+strings.Join(rendered, "; "))
	}
	if policy.Planning.RequireVerificationStep {
		requirement := strings.TrimSpace(options.VerificationRequirement)
		if requirement == "" {
			requirement = "Plans must include an explicit verification step."
		}
		lines = append(lines, requirement)
	}
	return strings.Join(lines, "\n")
}

// RenderExecutionPolicy renders execution-phase policy into a prompt string.
func RenderExecutionPolicy(policy *ResolvedAgentPolicy, stopOnSuccess bool) string {
	if policy == nil {
		return ""
	}
	var lines []string
	if successCapabilities := policy.VerificationSuccessCapabilities; len(successCapabilities) > 0 {
		lines = append(lines, "Verification success capabilities: "+strings.Join(successCapabilities, ", "))
	}
	if stopOnSuccess {
		lines = append(lines, "Stop immediately after a successful verification capability runs after the latest edit.")
	}
	if probes := policy.RecoveryProbeCapabilities; len(probes) > 0 {
		lines = append(lines, "Preferred recovery probes on failures: "+strings.Join(probes, ", "))
	}
	return strings.Join(lines, "\n")
}

// RenderReviewPolicy renders review-phase policy into a prompt string.
func RenderReviewPolicy(policy ResolvedAgentPolicy) string {
	var lines []string
	if len(policy.Review.Criteria) > 0 {
		lines = append(lines, "Review criteria: "+strings.Join(policy.Review.Criteria, ", "))
	} else {
		lines = append(lines, "Review criteria: correctness, completeness, quality, security, performance")
	}
	if len(policy.Review.FocusTags) > 0 {
		lines = append(lines, "Focus tags: "+strings.Join(policy.Review.FocusTags, ", "))
	}
	if policy.Review.ApprovalRules.RequireVerificationEvidence {
		lines = append(lines, "Require verification evidence before approval.")
	}
	if policy.Review.ApprovalRules.RejectOnUnresolvedErrors {
		lines = append(lines, "Reject outputs with unresolved errors.")
	}
	if summary := RenderSeverityWeights(policy.Review.SeverityWeights); summary != "" {
		lines = append(lines, summary)
	}
	return strings.Join(lines, "\n")
}

// RenderSeverityWeights formats severity weights into a prompt string.
func RenderSeverityWeights(weights map[string]float64) string {
	resolved := ResolveSeverityWeights(weights)
	return fmt.Sprintf("Severity weights: high=%.2f, medium=%.2f, low=%.2f. Approval only tolerates residual issues within the low-severity threshold.",
		resolved["high"], resolved["medium"], resolved["low"])
}

// ResolveSeverityWeights fills in default severity weights for any missing keys.
func ResolveSeverityWeights(input map[string]float64) map[string]float64 {
	weights := map[string]float64{
		"high":   1.0,
		"medium": 0.5,
		"low":    0.2,
	}
	for severity, value := range input {
		key := strings.ToLower(strings.TrimSpace(severity))
		if key == "" {
			continue
		}
		weights[key] = value
	}
	return weights
}
