package rust

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
)

func TestVerificationResolver_BuildPlan(t *testing.T) {
	resolver := NewVerificationResolver()
	plan, ok, err := resolver.BuildPlan(context.Background(), agentenv.VerificationPlanRequest{
		TaskInstruction:      "verify this Rust API change",
		Workspace:            ".",
		Files:                []string{"src/lib.rs", "Cargo.toml"},
		PublicSurfaceChanged: true,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !ok {
		t.Fatal("expected plan")
	}
	if plan.ScopeKind != "compatibility_sweep" {
		t.Fatalf("expected compatibility_sweep, got %q", plan.ScopeKind)
	}
	if len(plan.Commands) != 2 || plan.Commands[0].Command != "cargo" {
		t.Fatalf("expected cargo commands, got %#v", plan.Commands)
	}
}
