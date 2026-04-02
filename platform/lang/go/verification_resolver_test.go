package golang

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
)

func TestVerificationResolver_BuildPlan(t *testing.T) {
	resolver := NewVerificationResolver()
	plan, ok, err := resolver.BuildPlan(context.Background(), agentenv.VerificationPlanRequest{
		TaskInstruction:         "verify this Go change",
		Workspace:               ".",
		Files:                   []string{"named/euclo/runtime/verification.go"},
		TestFiles:               []string{"named/euclo/runtime/verification_test.go"},
		RequireVerificationStep: true,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ok {
		t.Fatal("expected plan")
	}
	if plan.ScopeKind != "package_tests" {
		t.Fatalf("expected package_tests, got %q", plan.ScopeKind)
	}
	if len(plan.Commands) != 2 {
		t.Fatalf("expected package test plus workspace sweep, got %#v", plan.Commands)
	}
}
