package capabilities

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/stretchr/testify/require"
)

func TestPlannerPlanDescriptor(t *testing.T) {
	cap := &plannerPlanCapability{env: testEnv(t)}
	desc := cap.Descriptor()
	require.Equal(t, "euclo:planner.plan", desc.ID)
	require.Equal(t, core.CapabilityRuntimeFamilyRelurpic, desc.RuntimeFamily)
	require.Contains(t, desc.Tags, "planning")
	profiles, ok := desc.Annotations["supported_profiles"].([]string)
	require.True(t, ok)
	require.Contains(t, profiles, "plan_stage_execute")
	require.Contains(t, profiles, "edit_verify_repair")
}

func TestPlannerPlanContract(t *testing.T) {
	cap := &plannerPlanCapability{env: testEnv(t)}
	contract := cap.Contract()
	require.Len(t, contract.RequiredInputs, 1)
	require.Equal(t, euclotypes.ArtifactKindIntake, contract.RequiredInputs[0].Kind)
	require.True(t, contract.RequiredInputs[0].Required)
	require.Len(t, contract.ProducedOutputs, 1)
	require.Equal(t, euclotypes.ArtifactKindPlan, contract.ProducedOutputs[0])
}

func TestPlannerPlanEligibleWithReadTools(t *testing.T) {
	cap := &plannerPlanCapability{env: testEnv(t)}
	snapshot := euclotypes.CapabilitySnapshot{HasReadTools: true}
	result := cap.Eligible(euclotypes.NewArtifactState(nil), snapshot)
	require.True(t, result.Eligible)
}

func TestPlannerPlanIneligibleWithoutReadTools(t *testing.T) {
	cap := &plannerPlanCapability{env: testEnv(t)}
	snapshot := euclotypes.CapabilitySnapshot{HasReadTools: false}
	result := cap.Eligible(euclotypes.NewArtifactState(nil), snapshot)
	require.False(t, result.Eligible)
	require.Contains(t, result.Reason, "read tools")
}

func TestPlannerPlanExecutePlanThenReflect(t *testing.T) {
	env := testEnv(t)
	cap := &plannerPlanCapability{env: env}
	state := core.NewContext()
	envelope := euclotypes.ExecutionEnvelope{
		Task:        &core.Task{ID: "test-plan", Instruction: "design the implementation"},
		Mode:        euclotypes.ModeResolution{ModeID: "planning"},
		Profile:     euclotypes.ExecutionProfileSelection{ProfileID: "plan_stage_execute"},
		Registry:    env.Registry,
		State:       state,
		Memory:      env.Memory,
		Environment: env,
	}

	result := cap.Execute(context.Background(), envelope)
	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, euclotypes.ArtifactKindPlan, result.Artifacts[0].Kind)
	require.Equal(t, "euclo:planner.plan", result.Artifacts[0].ProducerID)
	require.Equal(t, "produced", result.Artifacts[0].Status)
	require.Nil(t, result.FailureInfo)
}

func TestPlannerPlanRetryOnReflectionFeedback(t *testing.T) {
	// With the stub model, the reflection always returns a summary which
	// triggers the retry path. Verify the capability completes successfully
	// even with the retry.
	env := testEnv(t)
	cap := &plannerPlanCapability{env: env}
	state := core.NewContext()
	envelope := euclotypes.ExecutionEnvelope{
		Task:        &core.Task{ID: "test-plan-retry", Instruction: "plan with review"},
		Mode:        euclotypes.ModeResolution{ModeID: "planning"},
		Profile:     euclotypes.ExecutionProfileSelection{ProfileID: "plan_stage_execute"},
		Registry:    env.Registry,
		State:       state,
		Memory:      env.Memory,
		Environment: env,
	}

	result := cap.Execute(context.Background(), envelope)
	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, euclotypes.ArtifactKindPlan, result.Artifacts[0].Kind)
}

func TestPlannerPlanSupportedProfiles(t *testing.T) {
	cap := &plannerPlanCapability{env: testEnv(t)}
	desc := cap.Descriptor()
	profiles := desc.Annotations["supported_profiles"].([]string)
	require.Len(t, profiles, 2)
}
