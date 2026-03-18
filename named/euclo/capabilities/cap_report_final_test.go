package capabilities

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/stretchr/testify/require"
)

func TestReportFinalDescriptor(t *testing.T) {
	cap := &reportFinalCodingCapability{env: testEnv(t)}
	desc := cap.Descriptor()
	require.Equal(t, "euclo:report.final_coding", desc.ID)
	require.Equal(t, core.CapabilityRuntimeFamilyRelurpic, desc.RuntimeFamily)
	require.Contains(t, desc.Tags, "reporting")
}

func TestReportFinalContract(t *testing.T) {
	cap := &reportFinalCodingCapability{env: testEnv(t)}
	contract := cap.Contract()
	require.Len(t, contract.RequiredInputs, 3)
	for _, req := range contract.RequiredInputs {
		require.False(t, req.Required) // all optional
	}
	require.Len(t, contract.ProducedOutputs, 1)
	require.Equal(t, euclotypes.ArtifactKindFinalReport, contract.ProducedOutputs[0])
}

func TestReportFinalEligibleWithVerification(t *testing.T) {
	cap := &reportFinalCodingCapability{env: testEnv(t)}
	arts := euclotypes.NewArtifactState([]euclotypes.Artifact{{Kind: euclotypes.ArtifactKindVerification}})
	result := cap.Eligible(arts, euclotypes.CapabilitySnapshot{})
	require.True(t, result.Eligible)
}

func TestReportFinalEligibleWithPlan(t *testing.T) {
	cap := &reportFinalCodingCapability{env: testEnv(t)}
	arts := euclotypes.NewArtifactState([]euclotypes.Artifact{{Kind: euclotypes.ArtifactKindPlan}})
	result := cap.Eligible(arts, euclotypes.CapabilitySnapshot{})
	require.True(t, result.Eligible)
}

func TestReportFinalEligibleWithEditExecution(t *testing.T) {
	cap := &reportFinalCodingCapability{env: testEnv(t)}
	arts := euclotypes.NewArtifactState([]euclotypes.Artifact{{Kind: euclotypes.ArtifactKindEditExecution}})
	result := cap.Eligible(arts, euclotypes.CapabilitySnapshot{})
	require.True(t, result.Eligible)
}

func TestReportFinalIneligibleWithoutReportableArtifacts(t *testing.T) {
	cap := &reportFinalCodingCapability{env: testEnv(t)}
	arts := euclotypes.NewArtifactState([]euclotypes.Artifact{{Kind: euclotypes.ArtifactKindIntake}})
	result := cap.Eligible(arts, euclotypes.CapabilitySnapshot{})
	require.False(t, result.Eligible)
}

func TestReportFinalExecuteProducesReport(t *testing.T) {
	env := testEnv(t)
	cap := &reportFinalCodingCapability{env: env}

	state := core.NewContext()
	envelope := euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "test-report",
			Instruction: "generate final report",
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
	require.Equal(t, euclotypes.ArtifactKindFinalReport, result.Artifacts[0].Kind)
	require.Equal(t, "euclo:report.final_coding", result.Artifacts[0].ProducerID)
	require.Equal(t, "produced", result.Artifacts[0].Status)
}

func TestReportFinalSupportedProfilesAnnotation(t *testing.T) {
	cap := &reportFinalCodingCapability{env: testEnv(t)}
	desc := cap.Descriptor()
	profiles, ok := desc.Annotations["supported_profiles"].([]string)
	require.True(t, ok)
	require.Len(t, profiles, 6) // all profiles
}
