package euclo

import (
	"testing"

	"github.com/lexcodex/relurpify/ayenitd"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	golangpkg "github.com/lexcodex/relurpify/platform/lang/go"
)

func TestInitializeEnvironment_WiresDefaultVerificationPlanner(t *testing.T) {
	agent := &Agent{}
	err := agent.InitializeEnvironment(ayenitd.WorkspaceEnvironment{
		Registry: capability.NewRegistry(),
		Config:   &core.Config{Name: "test", Model: "stub", MaxIterations: 1},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if agent.Environment.VerificationPlanner == nil {
		t.Fatal("expected default verification planner to be wired")
	}
}

func TestInitializeEnvironment_PreservesExistingVerificationPlanner(t *testing.T) {
	custom := frameworkplan.NewVerificationScopePlanner(golangpkg.NewVerificationResolver())
	agent := &Agent{}
	err := agent.InitializeEnvironment(ayenitd.WorkspaceEnvironment{
		Registry:            capability.NewRegistry(),
		Config:              &core.Config{Name: "test", Model: "stub", MaxIterations: 1},
		VerificationPlanner: custom,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if agent.Environment.VerificationPlanner != custom {
		t.Fatal("expected existing verification planner to be preserved")
	}
}

func TestInitializeEnvironment_WiresCompatibilitySurfaceWhenNil(t *testing.T) {
	agent := &Agent{}
	if err := agent.InitializeEnvironment(ayenitd.WorkspaceEnvironment{
		Registry: capability.NewRegistry(),
		Config:   &core.Config{Name: "test", Model: "stub", MaxIterations: 1},
	}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if agent.Environment.CompatibilitySurfaceExtractor == nil {
		t.Fatal("expected default compatibility surface extractor to be wired")
	}
}
