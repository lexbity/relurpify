package toolsys

import (
	"fmt"
	"strings"
)

type PlanningRenderOptions struct {
	IncludePhaseTools          bool
	IncludeVerificationSuccess bool
	VerificationRequirement    string
}

func RenderPlanningPolicy(policy ResolvedSkillPolicy, options PlanningRenderOptions) string {
	var lines []string
	if options.IncludePhaseTools {
		if tools := policy.PhaseTools["explore"]; len(tools) > 0 {
			lines = append(lines, "Explore tools: "+strings.Join(tools, ", "))
		}
		if tools := policy.PhaseTools["edit"]; len(tools) > 0 {
			lines = append(lines, "Edit tools: "+strings.Join(tools, ", "))
		}
		if tools := policy.PhaseTools["verify"]; len(tools) > 0 {
			lines = append(lines, "Verify tools: "+strings.Join(tools, ", "))
		}
	}
	if options.IncludeVerificationSuccess {
		if tools := policy.VerificationSuccessTools; len(tools) > 0 {
			lines = append(lines, "Verification success tools: "+strings.Join(tools, ", "))
		}
	}
	if tools := policy.Planning.RequiredBeforeEdit; len(tools) > 0 {
		lines = append(lines, "Required before edit: "+strings.Join(tools, ", "))
	}
	if tools := policy.Planning.PreferredEditTools; len(tools) > 0 {
		lines = append(lines, "Preferred edit tools: "+strings.Join(tools, ", "))
	}
	if tools := policy.Planning.PreferredVerifyTools; len(tools) > 0 {
		lines = append(lines, "Preferred verify tools: "+strings.Join(tools, ", "))
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

func RenderExecutionPolicy(spec *ResolvedSkillPolicy, stopOnSuccess bool) string {
	if spec == nil {
		return ""
	}
	var lines []string
	if successTools := spec.VerificationSuccessTools; len(successTools) > 0 {
		lines = append(lines, "Verification success tools: "+strings.Join(successTools, ", "))
	}
	if stopOnSuccess {
		lines = append(lines, "Stop immediately after a successful verification tool runs after the latest edit.")
	}
	if probes := spec.RecoveryProbeTools; len(probes) > 0 {
		lines = append(lines, "Preferred recovery probes on failures: "+strings.Join(probes, ", "))
	}
	return strings.Join(lines, "\n")
}

func RenderReviewPolicy(policy ResolvedSkillPolicy) string {
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

func RenderSeverityWeights(weights map[string]float64) string {
	resolved := ResolveSeverityWeights(weights)
	return fmt.Sprintf("Severity weights: high=%.2f, medium=%.2f, low=%.2f. Approval only tolerates residual issues within the low-severity threshold.",
		resolved["high"], resolved["medium"], resolved["low"])
}

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
