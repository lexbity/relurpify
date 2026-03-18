package euclotest

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/named/euclo"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/internal/testutil"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	"github.com/stretchr/testify/require"
)

func TestRouteCapabilityFamiliesUsesModeAndProfileDeterministically(t *testing.T) {
	routing := eucloruntime.RouteCapabilityFamilies(
		euclotypes.ModeResolution{ModeID: "planning"},
		euclotypes.ExecutionProfileSelection{ProfileID: "plan_stage_execute", PhaseRoutes: map[string]string{"plan": "planner", "summarize": "react"}},
	)
	require.Equal(t, "planning", routing.PrimaryFamilyID)
	require.Equal(t, []string{"implementation"}, routing.FallbackFamilyIDs)
	require.NotEmpty(t, routing.Routes)
	require.Equal(t, "planning", routing.Routes[0].Family)
}

func TestRouteCapabilityFamiliesDebugMode(t *testing.T) {
	routing := eucloruntime.RouteCapabilityFamilies(
		euclotypes.ModeResolution{ModeID: "debug"},
		euclotypes.ExecutionProfileSelection{ProfileID: "reproduce_localize_patch", PhaseRoutes: map[string]string{"reproduce": "react", "localize": "react", "patch": "pipeline", "verify": "react"}},
	)
	require.Equal(t, "debugging", routing.PrimaryFamilyID)
	require.Equal(t, []string{"implementation", "verification"}, routing.FallbackFamilyIDs)
}

func TestRouteCapabilityFamiliesReviewMode(t *testing.T) {
	routing := eucloruntime.RouteCapabilityFamilies(
		euclotypes.ModeResolution{ModeID: "review"},
		euclotypes.ExecutionProfileSelection{ProfileID: "review_suggest_implement", PhaseRoutes: map[string]string{"review": "reflection", "summarize": "react"}},
	)
	require.Equal(t, "review", routing.PrimaryFamilyID)
	require.Equal(t, []string{"planning"}, routing.FallbackFamilyIDs)
}

func TestRouteCapabilityFamiliesDefaultMode(t *testing.T) {
	routing := eucloruntime.RouteCapabilityFamilies(
		euclotypes.ModeResolution{ModeID: "code"},
		euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair", PhaseRoutes: map[string]string{"explore": "react"}},
	)
	require.Equal(t, "implementation", routing.PrimaryFamilyID)
}

func TestRouteCapabilityFamiliesPhaseParadigmMapping(t *testing.T) {
	routing := eucloruntime.RouteCapabilityFamilies(
		euclotypes.ModeResolution{ModeID: "code"},
		euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair", PhaseRoutes: map[string]string{
			"explore": "react", "plan": "pipeline", "edit": "pipeline", "verify": "react",
		}},
	)
	// Routes should be sorted by phase.
	require.Len(t, routing.Routes, 4)
	// Plan phase should map to planning family with planner paradigm.
	for _, route := range routing.Routes {
		if route.Phase == "plan" {
			require.Equal(t, "planning", route.Family)
			require.Equal(t, "planner", route.Agent)
		}
		if route.Phase == "verify" {
			require.Equal(t, "verification", route.Family)
			require.Equal(t, "react", route.Agent)
		}
	}
}

func TestAgentPublishesCapabilityRoutingArtifact(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
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
	routing, ok := raw.(eucloruntime.CapabilityFamilyRouting)
	require.True(t, ok)
	require.Equal(t, "review", routing.PrimaryFamilyID)
}
