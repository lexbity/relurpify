package capabilities

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
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
	require.Equal(t, euclotypes.ArtifactKindIntake, contract.RequiredInputs[0].Kind)
	require.Len(t, contract.ProducedOutputs, 3)
	require.Contains(t, contract.ProducedOutputs, euclotypes.ArtifactKindPlan)
	require.Contains(t, contract.ProducedOutputs, euclotypes.ArtifactKindEditIntent)
	require.Contains(t, contract.ProducedOutputs, euclotypes.ArtifactKindVerification)
}

func TestTDDGenerateEligibleWithWriteAndVerifyTools(t *testing.T) {
	cap := &tddGenerateCapability{env: testEnv(t)}
	snapshot := euclotypes.CapabilitySnapshot{HasWriteTools: true, HasVerificationTools: true}
	result := cap.Eligible(euclotypes.NewArtifactState(nil), snapshot)
	require.True(t, result.Eligible)
}

func TestTDDGenerateIneligibleWithoutWriteTools(t *testing.T) {
	cap := &tddGenerateCapability{env: testEnv(t)}
	snapshot := euclotypes.CapabilitySnapshot{HasWriteTools: false, HasVerificationTools: true}
	result := cap.Eligible(euclotypes.NewArtifactState(nil), snapshot)
	require.False(t, result.Eligible)
	require.Contains(t, result.Reason, "write tools")
}

func TestTDDGenerateIneligibleWithoutVerificationTools(t *testing.T) {
	cap := &tddGenerateCapability{env: testEnv(t)}
	snapshot := euclotypes.CapabilitySnapshot{HasWriteTools: true, HasVerificationTools: false}
	result := cap.Eligible(euclotypes.NewArtifactState(nil), snapshot)
	require.False(t, result.Eligible)
	require.Contains(t, result.Reason, "verification tools")
}

func TestTDDGenerateExecuteProducesPlanEditAndVerificationArtifacts(t *testing.T) {
	env := testEnv(t)
	cap := &tddGenerateCapability{env: env}
	state := core.NewContext()
	envelope := euclotypes.ExecutionEnvelope{
		Task:        &core.Task{ID: "test-tdd", Instruction: "add tests and implement"},
		Mode:        euclotypes.ModeResolution{ModeID: "tdd"},
		Profile:     euclotypes.ExecutionProfileSelection{ProfileID: "test_driven_generation"},
		Registry:    env.Registry,
		State:       state,
		Memory:      env.Memory,
		Environment: env,
	}

	result := cap.Execute(context.Background(), envelope)
	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	require.Len(t, result.Artifacts, 3)
	require.Nil(t, result.FailureInfo)

	kinds := make(map[euclotypes.ArtifactKind]bool)
	for _, art := range result.Artifacts {
		require.Equal(t, "euclo:tdd.generate", art.ProducerID)
		require.Equal(t, "produced", art.Status)
		kinds[art.Kind] = true
	}
	require.True(t, kinds[euclotypes.ArtifactKindPlan])
	require.True(t, kinds[euclotypes.ArtifactKindEditIntent])
	require.True(t, kinds[euclotypes.ArtifactKindVerification])
}

func TestTDDGenerateInternalGateRequiresTestBeforeImplement(t *testing.T) {
	// Verify the capability produces plan before edit_intent in artifact order.
	env := testEnv(t)
	cap := &tddGenerateCapability{env: env}
	state := core.NewContext()
	envelope := euclotypes.ExecutionEnvelope{
		Task:        &core.Task{ID: "test-tdd-order", Instruction: "write tests first"},
		Mode:        euclotypes.ModeResolution{ModeID: "tdd"},
		Profile:     euclotypes.ExecutionProfileSelection{ProfileID: "test_driven_generation"},
		Registry:    env.Registry,
		State:       state,
		Memory:      env.Memory,
		Environment: env,
	}

	result := cap.Execute(context.Background(), envelope)
	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	// Plan (from HTN) comes before EditIntent (from implementation).
	require.Equal(t, euclotypes.ArtifactKindPlan, result.Artifacts[0].Kind)
	require.Equal(t, euclotypes.ArtifactKindEditIntent, result.Artifacts[1].Kind)
	require.Equal(t, euclotypes.ArtifactKindVerification, result.Artifacts[2].Kind)
}
