package capabilities

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/testutil/euclotestutil"
	"github.com/stretchr/testify/require"
)

func TestDesignAlternativesDescriptorAndEligibility(t *testing.T) {
	cap := &designAlternativesCapability{env: testEnv(t)}
	desc := cap.Descriptor()
	require.Equal(t, "euclo:design.alternatives", desc.ID)

	eligible := cap.Eligible(euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindIntake, Payload: map[string]any{"instruction": "Show alternatives for implementing this feature"}},
	}), euclotypes.CapabilitySnapshot{HasReadTools: true})
	require.True(t, eligible.Eligible)

	ineligible := cap.Eligible(euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindIntake, Payload: map[string]any{"instruction": "Implement this feature"}},
	}), euclotypes.CapabilitySnapshot{HasReadTools: true})
	require.False(t, ineligible.Eligible)
}

func TestDesignAlternativesExecuteProducesCandidatesAndPlan(t *testing.T) {
	env := testEnv(t)
	cap := &designAlternativesCapability{env: env}
	state := core.NewContext()
	state.Set("euclo.envelope", map[string]any{"instruction": "Show alternatives for fixing the build"})

	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task:        &core.Task{ID: "phase3-alt", Instruction: "Show alternatives for fixing the build"},
		Mode:        euclotypes.ModeResolution{ModeID: "planning"},
		Profile:     euclotypes.ExecutionProfileSelection{ProfileID: "plan_stage_execute"},
		Registry:    env.Registry,
		State:       state,
		Memory:      env.Memory,
		Environment: env,
	})

	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	require.Len(t, result.Artifacts, 2)
	require.Equal(t, euclotypes.ArtifactKindPlanCandidates, result.Artifacts[0].Kind)
	require.Equal(t, euclotypes.ArtifactKindPlan, result.Artifacts[1].Kind)

	payload, ok := result.Artifacts[0].Payload.(map[string]any)
	require.True(t, ok)
	candidates, ok := payload["candidates"].([]map[string]any)
	if !ok {
		raw, ok2 := payload["candidates"].([]any)
		require.True(t, ok2)
		require.Len(t, raw, 3)
	} else {
		require.Len(t, candidates, 3)
	}
	require.NotEmpty(t, payload["selected_id"])
}

func TestExecutionProfileSelectDescriptorEligibilityAndExecute(t *testing.T) {
	cap := &executionProfileSelectCapability{env: testEnv(t)}
	desc := cap.Descriptor()
	require.Equal(t, "euclo:execution_profile.select", desc.ID)

	eligible := cap.Eligible(euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindIntake, Payload: map[string]any{"instruction": "What profile should we use for this debug task?"}},
	}), euclotypes.CapabilitySnapshot{})
	require.True(t, eligible.Eligible)

	env := testutil.EnvMinimal()
	state := core.NewContext()
	state.Set("euclo.envelope", map[string]any{"instruction": "What profile should we use for this debug task?"})
	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task:        &core.Task{ID: "phase3-profile", Instruction: "What profile should we use for this debug task?"},
		Registry:    env.Registry,
		State:       state,
		Environment: env,
	})

	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, euclotypes.ArtifactKindProfileSelection, result.Artifacts[0].Kind)

	payload, ok := result.Artifacts[0].Payload.(map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, payload["selected_profile"])
	require.NotEmpty(t, payload["reasoning"])
}
