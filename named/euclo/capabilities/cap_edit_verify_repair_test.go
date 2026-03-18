package capabilities

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/internal/testutil"
	"github.com/stretchr/testify/require"
)

// testEnv is a backward-compatible wrapper for testutil.Env
func testEnv(t *testing.T) agentenv.AgentEnvironment {
	return testutil.Env(t)
}

func TestEditVerifyRepairDescriptor(t *testing.T) {
	cap := &editVerifyRepairCapability{env: testEnv(t)}
	desc := cap.Descriptor()
	require.Equal(t, "euclo:edit_verify_repair", desc.ID)
	require.Equal(t, core.CapabilityRuntimeFamilyRelurpic, desc.RuntimeFamily)
	require.Contains(t, desc.Tags, "coding")
}

func TestEditVerifyRepairContract(t *testing.T) {
	cap := &editVerifyRepairCapability{env: testEnv(t)}
	contract := cap.Contract()
	require.Len(t, contract.RequiredInputs, 1)
	require.Equal(t, euclotypes.ArtifactKindIntake, contract.RequiredInputs[0].Kind)
	require.True(t, contract.RequiredInputs[0].Required)
	require.Len(t, contract.ProducedOutputs, 4)
}

func TestEditVerifyRepairEligibleWithWriteAndVerifyTools(t *testing.T) {
	cap := &editVerifyRepairCapability{env: testEnv(t)}
	snapshot := euclotypes.CapabilitySnapshot{HasWriteTools: true, HasVerificationTools: true}
	result := cap.Eligible(euclotypes.NewArtifactState(nil), snapshot)
	require.True(t, result.Eligible)
}

func TestEditVerifyRepairIneligibleWithoutWriteTools(t *testing.T) {
	cap := &editVerifyRepairCapability{env: testEnv(t)}
	snapshot := euclotypes.CapabilitySnapshot{HasWriteTools: false, HasVerificationTools: true}
	result := cap.Eligible(euclotypes.NewArtifactState(nil), snapshot)
	require.False(t, result.Eligible)
	require.Contains(t, result.Reason, "write tools")
}

func TestEditVerifyRepairIneligibleWithoutVerificationTools(t *testing.T) {
	cap := &editVerifyRepairCapability{env: testEnv(t)}
	snapshot := euclotypes.CapabilitySnapshot{HasWriteTools: true, HasVerificationTools: false}
	result := cap.Eligible(euclotypes.NewArtifactState(nil), snapshot)
	require.False(t, result.Eligible)
	require.Contains(t, result.Reason, "verification tools")
}

func TestEditVerifyRepairExecuteProducesArtifacts(t *testing.T) {
	env := testEnv(t)
	cap := &editVerifyRepairCapability{env: env}

	state := core.NewContext()
	envelope := euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "test-evr",
			Instruction: "fix the bug",
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
	require.Len(t, result.Artifacts, 4)
	require.Nil(t, result.FailureInfo)
	require.Nil(t, result.RecoveryHint)

	// Check all artifacts have producer ID
	for _, art := range result.Artifacts {
		require.Equal(t, "euclo:edit_verify_repair", art.ProducerID)
		require.Equal(t, "produced", art.Status)
	}

	// Check artifact kinds
	kinds := make(map[euclotypes.ArtifactKind]bool)
	for _, art := range result.Artifacts {
		kinds[art.Kind] = true
	}
	require.True(t, kinds[euclotypes.ArtifactKindExplore])
	require.True(t, kinds[euclotypes.ArtifactKindPlan])
	require.True(t, kinds[euclotypes.ArtifactKindEditIntent])
	require.True(t, kinds[euclotypes.ArtifactKindVerification])
}

func TestEditVerifyRepairTaskContextFromEnvelope(t *testing.T) {
	env := euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "t1",
			Instruction: "do it",
			Context:     map[string]any{"workspace": "/tmp"},
		},
		Mode:    euclotypes.ModeResolution{ModeID: "code"},
		Profile: euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair"},
	}
	ctx := taskContextFrom(env)
	require.Equal(t, "code", ctx["mode"])
	require.Equal(t, "edit_verify_repair", ctx["profile"])
	require.Equal(t, "/tmp", ctx["workspace"])
}

func TestCapTaskInstructionFallback(t *testing.T) {
	require.Equal(t, "the requested change", capTaskInstruction(nil))
	require.Equal(t, "the requested change", capTaskInstruction(&core.Task{}))
	require.Equal(t, "fix it", capTaskInstruction(&core.Task{Instruction: "fix it"}))
}
