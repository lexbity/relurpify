package rewoo

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/capability"
)

// PreflightIssue describes a problem found during plan validation.
type PreflightIssue struct {
	Severity string // "error" or "warning"
	StepID   string
	Tool     string
	Message  string
}

// PreflightCheck validates a plan before execution.
// It checks:
// - All referenced tools exist in the registry
// - All tools have necessary permissions (if permission manager is available)
// - Plan structure is valid (steps, dependencies, etc.)
// Returns []PreflightIssue if problems found (empty if all OK).
func PreflightCheck(
	ctx context.Context,
	registry *capability.Registry,
	plan *RewooPlan,
	pm *authorization.PermissionManager,
) []PreflightIssue {
	var issues []PreflightIssue

	if plan == nil {
		return issues
	}

	if len(plan.Steps) == 0 {
		issues = append(issues, PreflightIssue{
			Severity: "warning",
			Message:  "plan has no steps",
		})
		return issues
	}

	// Track step IDs for dependency validation
	stepIDs := make(map[string]bool, len(plan.Steps))
	for _, step := range plan.Steps {
		stepIDs[step.ID] = true
	}

	for _, step := range plan.Steps {
		// Validate step ID is unique
		if step.ID == "" {
			issues = append(issues, PreflightIssue{
				Severity: "error",
				Message:  "step has empty ID",
			})
			continue
		}

		// Validate tool exists
		if step.Tool == "" {
			issues = append(issues, PreflightIssue{
				Severity: "error",
				StepID:   step.ID,
				Message:  "step references empty tool name",
			})
			continue
		}

		if registry != nil && !registry.HasCapability(step.Tool) {
			issues = append(issues, PreflightIssue{
				Severity: "error",
				StepID:   step.ID,
				Tool:     step.Tool,
				Message:  fmt.Sprintf("tool '%s' not found in registry", step.Tool),
			})
			continue
		}

		// Check tool permissions
		if pm != nil {
			if err := pm.CheckCapability(ctx, "rewoo", step.Tool); err != nil {
				issues = append(issues, PreflightIssue{
					Severity: "error",
					StepID:   step.ID,
					Tool:     step.Tool,
					Message:  fmt.Sprintf("permission denied: %v", err),
				})
			}
		}

		// Validate dependencies
		for _, dep := range step.DependsOn {
			if !stepIDs[dep] {
				issues = append(issues, PreflightIssue{
					Severity: "error",
					StepID:   step.ID,
					Message:  fmt.Sprintf("depends_on references unknown step '%s'", dep),
				})
			}
		}

		// Validate on_failure mode
		switch step.OnFailure {
		case "", StepOnFailureSkip, StepOnFailureAbort, StepOnFailureReplan:
			// Valid
		default:
			issues = append(issues, PreflightIssue{
				Severity: "warning",
				StepID:   step.ID,
				Message:  fmt.Sprintf("invalid on_failure mode: %s (using default)", step.OnFailure),
			})
		}
	}

	return issues
}

// IsValidPlan returns true if there are no error-level preflight issues.
func IsValidPlan(
	ctx context.Context,
	registry *capability.Registry,
	plan *RewooPlan,
	pm *authorization.PermissionManager,
) bool {
	issues := PreflightCheck(ctx, registry, plan, pm)
	for _, issue := range issues {
		if issue.Severity == "error" {
			return false
		}
	}
	return true
}
