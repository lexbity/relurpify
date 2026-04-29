package policy

import (
	"testing"
)

func TestEvaluatorEvaluate(t *testing.T) {
	evaluator := NewEvaluator()

	ctx := &PolicyContext{
		FamilyID:          "implementation",
		EditPermitted:     true,
		RequiresVerification: false,
		RiskLevel:         "medium",
		WorkspaceScopes:   []string{},
	}

	decision := evaluator.Evaluate(ctx)

	if decision == nil {
		t.Fatal("Expected decision to be non-nil")
	}

	if !decision.MutationPermitted {
		t.Error("Expected MutationPermitted to be true")
	}
}

func TestEvaluatorCheckPermission(t *testing.T) {
	evaluator := NewEvaluator()

	ctx := &PolicyContext{
		FamilyID:          "implementation",
		EditPermitted:     true,
		RequiresVerification: false,
		RiskLevel:         "medium",
		WorkspaceScopes:   []string{},
	}

	permitted := evaluator.CheckPermission(ctx)
	if !permitted {
		t.Error("Expected permission to be granted")
	}

	ctx.EditPermitted = false
	permitted = evaluator.CheckPermission(ctx)
	if permitted {
		t.Error("Expected permission to be denied")
	}
}

func TestEvaluatorRequestHITL(t *testing.T) {
	evaluator := NewEvaluator()

	ctx := &PolicyContext{
		FamilyID:          "debug",
		EditPermitted:     false,
		RequiresVerification: false,
		RiskLevel:         "low",
		WorkspaceScopes:   []string{},
	}

	required := evaluator.RequestHITL(ctx)
	if required {
		t.Error("Expected HITL not to be required for low risk")
	}

	ctx.RiskLevel = "high"
	required = evaluator.RequestHITL(ctx)
	if !required {
		t.Error("Expected HITL to be required for high risk")
	}
}
