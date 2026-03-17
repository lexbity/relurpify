package euclo

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestTDDGenerateDescriptor(t *testing.T) {
	cap := &tddGenerateCapability{env: testEnv(t)}
	desc := cap.Descriptor()
	require.Equal(t, "euclo:tdd.generate", desc.ID)
	require.Equal(t, core.CapabilityRuntimeFamilyRelurpic, desc.RuntimeFamily)
	require.Contains(t, desc.Tags, "tdd")
	profiles, ok := desc.Annotations["supported_profiles"].([]string)
	require.True(t, ok)
	require.Contains(t, profiles, "test_driven_generation")
}

func TestTDDGenerateContract(t *testing.T) {
	cap := &tddGenerateCapability{env: testEnv(t)}
	contract := cap.Contract()
	require.Len(t, contract.RequiredInputs, 1)
	require.Equal(t, ArtifactKindIntake, contract.RequiredInputs[0].Kind)
	require.Len(t, contract.ProducedOutputs, 3)
	require.Contains(t, contract.ProducedOutputs, ArtifactKindPlan)
	require.Contains(t, contract.ProducedOutputs, ArtifactKindEditIntent)
	require.Contains(t, contract.ProducedOutputs, ArtifactKindVerification)
}

func TestTDDGenerateEligibleWithWriteAndVerifyTools(t *testing.T) {
	cap := &tddGenerateCapability{env: testEnv(t)}
	snapshot := CapabilitySnapshot{HasWriteTools: true, HasVerificationTools: true}
	result := cap.Eligible(NewArtifactState(nil), snapshot)
	require.True(t, result.Eligible)
}

func TestTDDGenerateIneligibleWithoutWriteTools(t *testing.T) {
	cap := &tddGenerateCapability{env: testEnv(t)}
	snapshot := CapabilitySnapshot{HasWriteTools: false, HasVerificationTools: true}
	result := cap.Eligible(NewArtifactState(nil), snapshot)
	require.False(t, result.Eligible)
	require.Contains(t, result.Reason, "write tools")
}

func TestTDDGenerateIneligibleWithoutVerificationTools(t *testing.T) {
	cap := &tddGenerateCapability{env: testEnv(t)}
	snapshot := CapabilitySnapshot{HasWriteTools: true, HasVerificationTools: false}
	result := cap.Eligible(NewArtifactState(nil), snapshot)
	require.False(t, result.Eligible)
	require.Contains(t, result.Reason, "verification tools")
}

func TestTDDGenerateExecuteHTNToPipelineToReActFlow(t *testing.T) {
	env := testEnv(t)
	cap := &tddGenerateCapability{env: env}
	state := core.NewContext()
	envelope := ExecutionEnvelope{
		Task:        &core.Task{ID: "test-tdd", Instruction: "add tests and implement"},
		Mode:        ModeResolution{ModeID: "tdd"},
		Profile:     ExecutionProfileSelection{ProfileID: "test_driven_generation"},
		Registry:    env.Registry,
		State:       state,
		Memory:      env.Memory,
		Environment: env,
	}

	result := cap.Execute(context.Background(), envelope)
	require.Equal(t, ExecutionStatusCompleted, result.Status)
	require.Len(t, result.Artifacts, 3)
	require.Nil(t, result.FailureInfo)

	kinds := make(map[ArtifactKind]bool)
	for _, art := range result.Artifacts {
		require.Equal(t, "euclo:tdd.generate", art.ProducerID)
		require.Equal(t, "produced", art.Status)
		kinds[art.Kind] = true
	}
	require.True(t, kinds[ArtifactKindPlan])
	require.True(t, kinds[ArtifactKindEditIntent])
	require.True(t, kinds[ArtifactKindVerification])
}

func TestTDDGenerateInternalGateRequiresTestBeforeImplement(t *testing.T) {
	// Verify the capability produces plan before edit_intent in artifact order.
	env := testEnv(t)
	cap := &tddGenerateCapability{env: env}
	state := core.NewContext()
	envelope := ExecutionEnvelope{
		Task:        &core.Task{ID: "test-tdd-order", Instruction: "write tests first"},
		Mode:        ModeResolution{ModeID: "tdd"},
		Profile:     ExecutionProfileSelection{ProfileID: "test_driven_generation"},
		Registry:    env.Registry,
		State:       state,
		Memory:      env.Memory,
		Environment: env,
	}

	result := cap.Execute(context.Background(), envelope)
	require.Equal(t, ExecutionStatusCompleted, result.Status)
	// Plan (from HTN) comes before EditIntent (from implementation).
	require.Equal(t, ArtifactKindPlan, result.Artifacts[0].Kind)
	require.Equal(t, ArtifactKindEditIntent, result.Artifacts[1].Kind)
	require.Equal(t, ArtifactKindVerification, result.Artifacts[2].Kind)
}
