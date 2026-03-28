package capabilities

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/testutil/euclotestutil"
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

	raw, ok := state.Get("pipeline.verify")
	require.True(t, ok)
	payload, ok := raw.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "pass", payload["status"])
	checks, ok := payload["checks"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, checks, 1)
	require.Equal(t, "pass", checks[0]["status"])
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

func TestEditVerifyRepairForcedParadigmSwitchRecovery(t *testing.T) {
	env := testEnv(t)
	cap := &editVerifyRepairCapability{env: env}
	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "forced-evr-paradigm",
			Instruction: "force recovery",
			Context:     map[string]any{"euclo.force_recovery": "paradigm_switch"},
		},
		State:   core.NewContext(),
		Mode:    euclotypes.ModeResolution{ModeID: "code"},
		Profile: euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair"},
	})
	require.Equal(t, euclotypes.ExecutionStatusFailed, result.Status)
	require.NotNil(t, result.RecoveryHint)
	require.Equal(t, euclotypes.RecoveryStrategyParadigmSwitch, result.RecoveryHint.Strategy)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, "euclo:edit_verify_repair", result.Artifacts[0].ProducerID)
}

func TestEditVerifyRepairForcedModeEscalationRecovery(t *testing.T) {
	env := testEnv(t)
	cap := &editVerifyRepairCapability{env: env}
	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "forced-evr-mode",
			Instruction: "force recovery",
			Context:     map[string]any{"euclo.force_recovery": "mode_escalation"},
		},
		State:   core.NewContext(),
		Mode:    euclotypes.ModeResolution{ModeID: "code"},
		Profile: euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair"},
	})
	require.Equal(t, euclotypes.ExecutionStatusPartial, result.Status)
	require.NotNil(t, result.RecoveryHint)
	require.Equal(t, euclotypes.RecoveryStrategyModeEscalation, result.RecoveryHint.Strategy)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, "euclo:edit_verify_repair", result.Artifacts[0].ProducerID)
}

func TestEditVerifyRepairForcedRecoveryActiveCompletes(t *testing.T) {
	env := testEnv(t)
	state := core.NewContext()
	cap := &editVerifyRepairCapability{env: env}
	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "forced-evr-recovery-active",
			Instruction: "force recovery",
			Context: map[string]any{
				"euclo.force_recovery":  "paradigm_switch",
				"euclo.recovery_active": true,
			},
		},
		State:   state,
		Mode:    euclotypes.ModeResolution{ModeID: "code"},
		Profile: euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair"},
	})
	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	require.Len(t, result.Artifacts, 3)
	raw, ok := state.Get("pipeline.verify")
	require.True(t, ok)
	payload, ok := raw.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "pass", payload["status"])
}

func TestCapTaskInstructionFallback(t *testing.T) {
	require.Equal(t, "the requested change", capTaskInstruction(nil))
	require.Equal(t, "the requested change", capTaskInstruction(&core.Task{}))
	require.Equal(t, "fix it", capTaskInstruction(&core.Task{Instruction: "fix it"}))
}

type verificationStubTool struct {
	name string
}

func (t verificationStubTool) Name() string                     { return t.name }
func (t verificationStubTool) Description() string              { return "stub verification tool" }
func (t verificationStubTool) Category() string                 { return "test" }
func (t verificationStubTool) Parameters() []core.ToolParameter { return nil }
func (t verificationStubTool) IsAvailable(context.Context, *core.Context) bool {
	return true
}
func (t verificationStubTool) Permissions() core.ToolPermissions { return core.ToolPermissions{} }
func (t verificationStubTool) Tags() []string                    { return []string{core.TagExecute} }
func (t verificationStubTool) Execute(_ context.Context, _ *core.Context, _ map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"summary": "tests passed",
		},
	}, nil
}

func TestVerificationFallbackPayloadUsesGoTest(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(verificationStubTool{name: "go_test"}))

	state := core.NewContext()
	state.Set("pipeline.code", map[string]any{
		"final_output": map[string]any{
			"result": map[string]any{
				"file_write": map[string]any{
					"data": map[string]any{
						"path": "/tmp/ws/testsuite/fixtures/div.go",
					},
				},
			},
		},
	})

	payload, ok := verificationFallbackPayload(context.Background(), euclotypes.ExecutionEnvelope{
		Task:     &core.Task{Context: map[string]any{"workspace": "/tmp/ws"}},
		Registry: registry,
		State:    state,
	})
	require.True(t, ok)
	require.Equal(t, "pass", payload["status"])
	checks, ok := payload["checks"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, checks, 1)
	require.Equal(t, "go_test", checks[0]["name"])
}

func TestVerificationFallbackPayloadUsesExecRunTests(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(verificationStubTool{name: "exec_run_tests"}))

	state := core.NewContext()
	state.Set("pipeline.code", map[string]any{
		"final_output": map[string]any{
			"result": map[string]any{
				"file_write": map[string]any{
					"data": map[string]any{
						"path": "/tmp/ws/testsuite/agenttest_fixtures/pysuite/calc.py",
					},
				},
			},
		},
	})

	payload, ok := verificationFallbackPayload(context.Background(), euclotypes.ExecutionEnvelope{
		Task:     &core.Task{Context: map[string]any{"workspace": "/tmp/ws"}},
		Registry: registry,
		State:    state,
	})
	require.True(t, ok)
	require.Equal(t, "pass", payload["status"])
	checks, ok := payload["checks"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, checks, 1)
	require.Equal(t, "exec_run_tests", checks[0]["name"])
}
