package js

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
)

func TestVerificationResolver_BuildPlan(t *testing.T) {
	resolver := NewVerificationResolver()
	plan, ok, err := resolver.BuildPlan(context.Background(), agentenv.VerificationPlanRequest{
		TaskInstruction: "verify this TypeScript change",
		Workspace:       ".",
		Files:           []string{"src/app.ts", "package.json"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ok {
		t.Fatal("expected plan")
	}
	if plan.ScopeKind != "workspace_tests" {
		t.Fatalf("expected workspace_tests, got %q", plan.ScopeKind)
	}
	if len(plan.Commands) != 1 || plan.Commands[0].Command != "npm" {
		t.Fatalf("expected npm test command, got %#v", plan.Commands)
	}
}
