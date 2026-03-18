package capabilities

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/stretchr/testify/require"
)

func TestVerifyChangeDescriptor(t *testing.T) {
	cap := &verifyChangeCapability{env: testEnv(t)}
	desc := cap.Descriptor()
	require.Equal(t, "euclo:verify.change", desc.ID)
	require.Equal(t, core.CapabilityRuntimeFamilyRelurpic, desc.RuntimeFamily)
}

func TestVerifyChangeContract(t *testing.T) {
	cap := &verifyChangeCapability{env: testEnv(t)}
	contract := cap.Contract()
	require.Len(t, contract.RequiredInputs, 2)
	require.False(t, contract.RequiredInputs[0].Required) // optional inputs
	require.False(t, contract.RequiredInputs[1].Required)
	require.Len(t, contract.ProducedOutputs, 1)
	require.Equal(t, euclotypes.ArtifactKindVerification, contract.ProducedOutputs[0])
}

func TestVerifyChangeEligibleWithEditIntent(t *testing.T) {
	cap := &verifyChangeCapability{env: testEnv(t)}
	arts := euclotypes.NewArtifactState([]euclotypes.Artifact{{Kind: euclotypes.ArtifactKindEditIntent}})
	result := cap.Eligible(arts, euclotypes.CapabilitySnapshot{})
	require.True(t, result.Eligible)
}

func TestVerifyChangeEligibleWithEditExecution(t *testing.T) {
	cap := &verifyChangeCapability{env: testEnv(t)}
	arts := euclotypes.NewArtifactState([]euclotypes.Artifact{{Kind: euclotypes.ArtifactKindEditExecution}})
	result := cap.Eligible(arts, euclotypes.CapabilitySnapshot{})
	require.True(t, result.Eligible)
}

func TestVerifyChangeIneligibleWithoutEdits(t *testing.T) {
	cap := &verifyChangeCapability{env: testEnv(t)}
	arts := euclotypes.NewArtifactState([]euclotypes.Artifact{{Kind: euclotypes.ArtifactKindPlan}})
	result := cap.Eligible(arts, euclotypes.CapabilitySnapshot{})
	require.False(t, result.Eligible)
}

func TestVerifyChangeExecuteProducesVerification(t *testing.T) {
	env := testEnv(t)
	cap := &verifyChangeCapability{env: env}

	state := core.NewContext()
	envelope := euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "test-verify",
			Instruction: "verify changes",
		},
		Mode:        euclotypes.ModeResolution{ModeID: "code"},
		Profile:     euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair"},
		Registry:    env.Registry,
		State:       state,
		Memory:      env.Memory,
		Environment: env,
	}

	result := cap.Execute(context.Background(), envelope)
	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, euclotypes.ArtifactKindVerification, result.Artifacts[0].Kind)
	require.Equal(t, "euclo:verify.change", result.Artifacts[0].ProducerID)
	require.Equal(t, "produced", result.Artifacts[0].Status)
}
