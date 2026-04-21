package local

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
	testutil "codeburg.org/lexbit/relurpify/testutil/euclotestutil"
)

// readScopeTool is a minimal tool with filesystem read permission so SnapshotCapabilities sets HasReadTools.
type readScopeTool struct{}

func (readScopeTool) Name() string                     { return "file_read" }
func (readScopeTool) Description() string              { return "read files" }
func (readScopeTool) Category() string                 { return "test" }
func (readScopeTool) Parameters() []core.ToolParameter { return nil }
func (readScopeTool) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (readScopeTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (readScopeTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "."}},
	}}
}
func (readScopeTool) Tags() []string { return nil }

func TestDesignAlternativesEligibleRequiresAlternativesIntent(t *testing.T) {
	env := testutil.Env(t)
	cap := NewDesignAlternativesCapability(env)
	snap := eucloruntime.SnapshotCapabilities(testutil.RegistryWith(readScopeTool{}))
	boring := euclotypes.NewArtifactState([]euclotypes.Artifact{{
		Kind:    euclotypes.ArtifactKindIntake,
		Payload: map[string]any{"instruction": "implement the handler"},
	}})
	if cap.Eligible(boring, snap).Eligible {
		t.Fatal("expected ineligible without alternatives phrasing")
	}
	good := euclotypes.NewArtifactState([]euclotypes.Artifact{{
		Kind:    euclotypes.ArtifactKindIntake,
		Payload: map[string]any{"instruction": "compare approaches for layering"},
	}})
	if !cap.Eligible(good, snap).Eligible {
		t.Fatal("expected eligible for comparison intent")
	}
}

func TestDesignAlternativesExecuteProducesCandidatesWithStubPlanner(t *testing.T) {
	env := testutil.Env(t)
	env.Config.MaxIterations = 2
	env.Registry = testutil.RegistryWith(readScopeTool{})
	state := core.NewContext()
	state.Set("euclo.artifacts", []euclotypes.Artifact{{
		Kind:    euclotypes.ArtifactKindIntake,
		Payload: map[string]any{"instruction": "Which approach should we take? options for API versioning"},
	}})
	result := NewDesignAlternativesCapability(env).Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "design-1",
			Instruction: "Which approach should we take? options for API versioning",
			Context:     map[string]any{"workspace": "."},
		},
		State:       state,
		Environment: env,
		Registry:    env.Registry,
	})
	if result.Status != euclotypes.ExecutionStatusCompleted {
		t.Fatalf("expected completed design alternatives, got %+v", result)
	}
	if len(result.Artifacts) == 0 {
		t.Fatal("expected artifacts")
	}
	foundCandidates := false
	for _, a := range result.Artifacts {
		if a.Kind == euclotypes.ArtifactKindPlanCandidates {
			foundCandidates = true
			break
		}
	}
	if !foundCandidates {
		t.Fatalf("expected plan candidates artifact, got %#v", result.Artifacts)
	}
}
