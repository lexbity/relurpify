package policy

import (
	"testing"
)

func TestEvaluateRouteMutatingFamily(t *testing.T) {
	ctx := &PolicyContext{
		FamilyID:          "implementation",
		EditPermitted:     true,
		RequiresVerification: false,
		RiskLevel:         "medium",
		WorkspaceScopes:   []string{},
	}

	decision := EvaluateRoute(ctx)

	if !decision.MutationPermitted {
		t.Error("Expected MutationPermitted to be true for mutating family with EditPermitted")
	}

	if !decision.HITLRequired {
		t.Error("Expected HITLRequired to be true (implementation is mutating)")
	}

	if decision.VerificationRequired {
		t.Error("Expected VerificationRequired to be false")
	}
}

func TestEvaluateRouteMutatingFamilyNoEditPermission(t *testing.T) {
	ctx := &PolicyContext{
		FamilyID:          "implementation",
		EditPermitted:     false,
		RequiresVerification: false,
		RiskLevel:         "medium",
		WorkspaceScopes:   []string{},
	}

	decision := EvaluateRoute(ctx)

	if decision.MutationPermitted {
		t.Error("Expected MutationPermitted to be false for mutating family without EditPermitted")
	}

	if !decision.HITLRequired {
		t.Error("Expected HITLRequired to be true (implementation is mutating)")
	}
}

func TestEvaluateRouteReadOnlyFamily(t *testing.T) {
	ctx := &PolicyContext{
		FamilyID:          "debug",
		EditPermitted:     false,
		RequiresVerification: false,
		RiskLevel:         "low",
		WorkspaceScopes:   []string{},
	}

	decision := EvaluateRoute(ctx)

	if !decision.MutationPermitted {
		t.Error("Expected MutationPermitted to be true for read-only family")
	}

	if decision.HITLRequired {
		t.Error("Expected HITLRequired to be false for read-only family")
	}
}

func TestEvaluateRouteHighRisk(t *testing.T) {
	ctx := &PolicyContext{
		FamilyID:          "migration",
		EditPermitted:     true,
		RequiresVerification: false,
		RiskLevel:         "high",
		WorkspaceScopes:   []string{},
	}

	decision := EvaluateRoute(ctx)

	if !decision.HITLRequired {
		t.Error("Expected HITLRequired to be true for high risk")
	}

	if !decision.MutationPermitted {
		t.Error("Expected MutationPermitted to be true for migration with EditPermitted")
	}
}

func TestEvaluateRouteVerificationRequired(t *testing.T) {
	ctx := &PolicyContext{
		FamilyID:          "debug",
		EditPermitted:     false,
		RequiresVerification: true,
		RiskLevel:         "low",
		WorkspaceScopes:   []string{},
	}

	decision := EvaluateRoute(ctx)

	if !decision.VerificationRequired {
		t.Error("Expected VerificationRequired to be true")
	}
}

func TestEvaluateRouteReasonCodes(t *testing.T) {
	ctx := &PolicyContext{
		FamilyID:          "implementation",
		EditPermitted:     true,
		RequiresVerification: true,
		RiskLevel:         "high",
		WorkspaceScopes:   []string{},
	}

	decision := EvaluateRoute(ctx)

	if len(decision.ReasonCodes) == 0 {
		t.Error("Expected reason codes to be populated")
	}

	hasEditPermitted := false
	hasHighRisk := false
	hasVerification := false

	for _, code := range decision.ReasonCodes {
		if code == "edit_permitted" {
			hasEditPermitted = true
		}
		if code == "high_risk" {
			hasHighRisk = true
		}
		if code == "verification_required" {
			hasVerification = true
		}
	}

	if !hasEditPermitted {
		t.Error("Expected edit_permitted reason code")
	}

	if !hasHighRisk {
		t.Error("Expected high_risk reason code")
	}

	if !hasVerification {
		t.Error("Expected verification_required reason code")
	}
}

func TestIsMutatingFamily(t *testing.T) {
	mutating := []string{"implementation", "refactor", "repair", "migration"}
	nonMutating := []string{"debug", "review", "planning", "investigation", "architecture"}

	for _, family := range mutating {
		if !isMutatingFamily(family) {
			t.Errorf("Expected %s to be mutating", family)
		}
	}

	for _, family := range nonMutating {
		if isMutatingFamily(family) {
			t.Errorf("Expected %s to be non-mutating", family)
		}
	}
}
