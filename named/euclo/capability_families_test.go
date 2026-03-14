package euclo

import (
	"context"
	"testing"

	plannerpkg "github.com/lexcodex/relurpify/agents/planner"
	reactpkg "github.com/lexcodex/relurpify/agents/react"
	reflectionpkg "github.com/lexcodex/relurpify/agents/reflection"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/stretchr/testify/require"
)

func TestRouteCapabilityFamiliesUsesModeAndProfileDeterministically(t *testing.T) {
	routing := RouteCapabilityFamilies(
		ModeResolution{ModeID: "planning"},
		ExecutionProfileSelection{ProfileID: "plan_stage_execute", PhaseRoutes: map[string]string{"plan": "planner", "summarize": "react"}},
		DefaultCapabilityFamilyRegistry(),
	)
	require.Equal(t, "planning", routing.PrimaryFamilyID)
	require.Equal(t, []string{"implementation"}, routing.FallbackFamilyIDs)
	require.NotEmpty(t, routing.Routes)
	require.Equal(t, "planning", routing.Routes[0].Family)
}

func TestBuildExecutorForRoutingChoosesPlannerAndReviewer(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	agent := New(agentenv.AgentEnvironment{
		Model:    eucloStubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-routing", Model: "stub", MaxIterations: 1},
	})

	executor, err := agent.buildExecutorForRouting(CapabilityFamilyRouting{ModeID: "planning", PrimaryFamilyID: "planning"})
	require.NoError(t, err)
	require.IsType(t, &plannerpkg.PlannerAgent{}, executor)

	executor, err = agent.buildExecutorForRouting(CapabilityFamilyRouting{ModeID: "review", PrimaryFamilyID: "review"})
	require.NoError(t, err)
	require.IsType(t, &reflectionpkg.ReflectionAgent{}, executor)

	executor, err = agent.buildExecutorForRouting(CapabilityFamilyRouting{ModeID: "code", PrimaryFamilyID: "implementation"})
	require.NoError(t, err)
	require.IsType(t, &reactpkg.ReActAgent{}, executor)
}

func TestAgentPublishesCapabilityRoutingArtifact(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	agent := New(agentenv.AgentEnvironment{
		Model:    eucloStubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-routing", Model: "stub", MaxIterations: 1},
	})

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verified",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	_, err = agent.Execute(context.Background(), &core.Task{
		ID:          "task-routing",
		Instruction: "review the current changes",
		Context:     map[string]any{"mode": "review"},
	}, state)
	require.NoError(t, err)

	raw, ok := state.Get("euclo.capability_family_routing")
	require.True(t, ok)
	routing, ok := raw.(CapabilityFamilyRouting)
	require.True(t, ok)
	require.Equal(t, "review", routing.PrimaryFamilyID)
}
