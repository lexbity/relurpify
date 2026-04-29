package policy

// EvaluateRoute derives a PolicyDecision from policy context.
// It determines whether mutation is permitted, whether HITL is required,
// and whether verification is required based on family and intent classification.
func EvaluateRoute(ctx *PolicyContext) *PolicyDecision {
	decision := &PolicyDecision{
		MutationPermitted:    false,
		HITLRequired:         false,
		VerificationRequired: false,
		ReasonCodes:          []string{},
	}

	// Mutating families require edit permission
	if isMutatingFamily(ctx.FamilyID) {
		decision.MutationPermitted = ctx.EditPermitted
		if ctx.EditPermitted {
			decision.ReasonCodes = append(decision.ReasonCodes, "edit_permitted")
		} else {
			decision.ReasonCodes = append(decision.ReasonCodes, "edit_not_permitted")
		}
	} else {
		// Non-mutating families are always permitted
		decision.MutationPermitted = true
		decision.ReasonCodes = append(decision.ReasonCodes, "read_only_family")
	}

	// HITL is required for mutating families or high-risk operations
	if isMutatingFamily(ctx.FamilyID) {
		decision.HITLRequired = true
		decision.ReasonCodes = append(decision.ReasonCodes, "mutating_family")
	}
	if ctx.RiskLevel == "high" {
		decision.HITLRequired = true
		decision.ReasonCodes = append(decision.ReasonCodes, "high_risk")
	}

	// Verification requirement from intent classification
	decision.VerificationRequired = ctx.RequiresVerification
	if ctx.RequiresVerification {
		decision.ReasonCodes = append(decision.ReasonCodes, "verification_required")
	}

	return decision
}

// isMutatingFamily checks if a family is mutating (makes edits to code).
func isMutatingFamily(familyID string) bool {
	mutatingFamilies := map[string]bool{
		"implementation": true,
		"refactor":       true,
		"repair":         true,
		"migration":      true,
	}
	return mutatingFamilies[familyID]
}
