package euclotest

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/capabilities"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/gate"
	"github.com/lexcodex/relurpify/named/euclo/orchestrate"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFullExecuteCodeModeEditVerifyRepair(t *testing.T) {
	agent := integrationAgent(t)

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "all tests pass",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})

	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "int-code-1",
		Instruction: "implement the foo feature",
		Context:     map[string]any{"workspace": "/tmp/ws"},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Mode resolution should have happened.
	raw, ok := state.Get("euclo.mode_resolution")
	require.True(t, ok)
	mode, ok := raw.(euclotypes.ModeResolution)
	require.True(t, ok)
	assert.NotEmpty(t, mode.ModeID)

	// Profile should be selected.
	raw, ok = state.Get("euclo.execution_profile_selection")
	require.True(t, ok)
	profile, ok := raw.(euclotypes.ExecutionProfileSelection)
	require.True(t, ok)
	assert.NotEmpty(t, profile.ProfileID)

	// Capability family routing artifact present.
	raw, ok = state.Get("euclo.capability_family_routing")
	require.True(t, ok)
	routing, ok := raw.(eucloruntime.CapabilityFamilyRouting)
	require.True(t, ok)
	assert.NotEmpty(t, routing.PrimaryFamilyID)

	// Action log and proof surface should be populated.
	raw, ok = state.Get("euclo.action_log")
	require.True(t, ok)
	actionLog, ok := raw.([]eucloruntime.ActionLogEntry)
	require.True(t, ok)
	assert.NotEmpty(t, actionLog)

	raw, ok = state.Get("euclo.proof_surface")
	require.True(t, ok)
	proof, ok := raw.(eucloruntime.ProofSurface)
	require.True(t, ok)
	assert.NotEmpty(t, proof.ArtifactKinds)

	// Session ID should be stored.
	sessionID := state.GetString("euclo.session_id")
	assert.NotEmpty(t, sessionID)
	assert.Contains(t, sessionID, "euclo-")
}

func TestFullExecuteDebugModeReproduceLocalizePatch(t *testing.T) {
	agent := integrationAgent(t)

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification ok",
		"checks":  []any{map[string]any{"name": "go test", "status": "pass"}},
	})

	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "int-debug-1",
		Instruction: "fix the failing test in pkg/foo",
		Context:     map[string]any{"mode": "debug", "workspace": "/tmp/ws"},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)

	raw, ok := state.Get("euclo.mode_resolution")
	require.True(t, ok)
	mode, ok := raw.(euclotypes.ModeResolution)
	require.True(t, ok)
	assert.Equal(t, "debug", mode.ModeID)

	// Profile controller should have been invoked.
	raw, ok = state.Get("euclo.profile_controller")
	assert.True(t, ok)
}

func TestFullExecuteWithRecoveryFallback(t *testing.T) {
	// Test the full recovery path through ProfileController + RecoveryController.
	// Uses ProfileController.ExecuteProfile directly to avoid mode/profile
	// resolution indirection.
	reg := capabilities.NewEucloCapabilityRegistry()
	primary := &stubProfileCapability{
		id:       "euclo:primary_fails",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "primary failed",
			RecoveryHint: &euclotypes.RecoveryHint{
				Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
				SuggestedCapability: "euclo:recovery_succeeds",
			},
		},
	}
	recoveryCap := &stubProfileCapability{
		id:       "euclo:recovery_succeeds",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusCompleted,
			Summary: "recovered",
			Artifacts: []euclotypes.Artifact{
				{Kind: euclotypes.ArtifactKindEditIntent, ProducerID: "euclo:recovery_succeeds", Status: "produced"},
				{Kind: euclotypes.ArtifactKindVerification, ProducerID: "euclo:recovery_succeeds", Status: "produced"},
			},
		},
	}
	_ = reg.Register(primary)
	_ = reg.Register(recoveryCap)

	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(orchestrate.AdaptCapabilityRegistry(reg), euclotypes.DefaultExecutionProfileRegistry(), euclotypes.DefaultModeRegistry(), env)
	pc := orchestrate.NewProfileController(orchestrate.AdaptCapabilityRegistry(reg), gate.DefaultPhaseGates(), env, euclotypes.DefaultExecutionProfileRegistry(), rc)

	profile := euclotypes.ExecutionProfileSelection{
		ProfileID:   "edit_verify_repair",
		PhaseRoutes: map[string]string{"explore": "react", "plan": "pipeline", "edit": "pipeline", "verify": "react"},
	}
	mode := euclotypes.ModeResolution{ModeID: "code"}

	result, pcResult, err := pc.ExecuteProfile(context.Background(), profile, mode, testEnvelope(nil))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	// Recovery should have been attempted and succeeded.
	assert.True(t, recoveryCap.executeCalled, "recovery capability should have been executed")
	assert.True(t, primary.executeCalled, "primary capability should have been executed")
	assert.Equal(t, 1, pcResult.RecoveryAttempts, "should have 1 recovery attempt")

	// Recovery trace artifact should be present.
	hasRecoveryTrace := false
	for _, art := range pcResult.Artifacts {
		if art.Kind == euclotypes.ArtifactKindRecoveryTrace {
			hasRecoveryTrace = true
		}
	}
	assert.True(t, hasRecoveryTrace, "should have recovery trace artifact")
}

func TestFullExecuteSessionScopingPreventsRecursion(t *testing.T) {
	agent := integrationAgent(t)

	// Pre-set a different session ID to simulate recursive invocation.
	state := core.NewContext()
	state.Set("euclo.session_id", "euclo-existing-session")
	state.Set("pipeline.verify", map[string]any{
		"status": "pass", "summary": "ok",
		"checks": []any{map[string]any{"name": "test", "status": "pass"}},
	})

	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "int-recursion-1",
		Instruction: "should fail due to session scoping",
	}, state)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "session scoping violation")
	require.NotNil(t, result)
	assert.False(t, result.Success)
}

func TestFullExecuteArtifactProvenanceComplete(t *testing.T) {
	// Test artifact provenance through ProfileController directly.
	reg := capabilities.NewEucloCapabilityRegistry()
	provCap := &stubProfileCapability{
		id:       "euclo:provenance_test",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusCompleted,
			Summary: "done",
			Artifacts: []euclotypes.Artifact{
				{Kind: euclotypes.ArtifactKindExplore, ProducerID: "euclo:provenance_test", Status: "produced"},
				{Kind: euclotypes.ArtifactKindPlan, ProducerID: "euclo:provenance_test", Status: "produced"},
				{Kind: euclotypes.ArtifactKindEditIntent, ProducerID: "euclo:provenance_test", Status: "produced"},
			},
		},
	}
	_ = reg.Register(provCap)

	env := testEnv(t)
	pc := orchestrate.NewProfileController(orchestrate.AdaptCapabilityRegistry(reg), gate.DefaultPhaseGates(), env, euclotypes.DefaultExecutionProfileRegistry(), nil)

	profile := euclotypes.ExecutionProfileSelection{
		ProfileID:   "edit_verify_repair",
		PhaseRoutes: map[string]string{"explore": "react", "plan": "pipeline", "edit": "pipeline", "verify": "react"},
	}
	mode := euclotypes.ModeResolution{ModeID: "code"}

	result, pcResult, err := pc.ExecuteProfile(context.Background(), profile, mode, testEnvelope(nil))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	// All capability-produced artifacts should have ProducerID.
	capProduced := 0
	for _, a := range pcResult.Artifacts {
		if a.ProducerID == "euclo:provenance_test" {
			capProduced++
		}
	}
	assert.GreaterOrEqual(t, capProduced, 3, "expected at least 3 capability-produced artifacts")

	// ValidateArtifactProvenance should find no warnings.
	warnings := euclotypes.ValidateArtifactProvenance(pcResult.Artifacts)
	assert.Empty(t, warnings, "all produced artifacts should have ProducerID")
}

func TestValidateArtifactProvenanceDetectsMissingProducerID(t *testing.T) {
	artifacts := []euclotypes.Artifact{
		{ID: "a", Kind: euclotypes.ArtifactKindExplore, Status: "produced", ProducerID: "cap_a"},
		{ID: "b", Kind: euclotypes.ArtifactKindPlan, Status: "produced", ProducerID: ""},
		{ID: "c", Kind: euclotypes.ArtifactKindEditIntent, Status: "pending", ProducerID: ""},
	}
	warnings := euclotypes.ValidateArtifactProvenance(artifacts)
	assert.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "artifact b")
}

func TestValidateArtifactProvenanceAllGood(t *testing.T) {
	artifacts := []euclotypes.Artifact{
		{ID: "a", Kind: euclotypes.ArtifactKindExplore, Status: "produced", ProducerID: "cap_a"},
		{ID: "b", Kind: euclotypes.ArtifactKindPlan, Status: "produced", ProducerID: "cap_b"},
	}
	warnings := euclotypes.ValidateArtifactProvenance(artifacts)
	assert.Empty(t, warnings)
}

func TestValidateArtifactProvenanceEmpty(t *testing.T) {
	warnings := euclotypes.ValidateArtifactProvenance(nil)
	assert.Empty(t, warnings)
}

// TestFamilyForPhase tests the familyForPhase function from runtime/routing.go
// Note: familyForPhase is a private function in the runtime package and cannot be called directly from tests.
// This test is commented out pending refactoring to expose the function or move to runtime_test.go
/*
func TestFamilyForPhase(t *testing.T) {
	assert.Equal(t, "planning", familyForPhase("plan", "implementation"))
	assert.Equal(t, "planning", familyForPhase("plan_tests", "implementation"))
	assert.Equal(t, "review", familyForPhase("review", "implementation"))
	assert.Equal(t, "review", familyForPhase("summarize", "implementation"))
	assert.Equal(t, "verification", familyForPhase("verify", "implementation"))
	assert.Equal(t, "debugging", familyForPhase("reproduce", "debugging"))
	assert.Equal(t, "debugging", familyForPhase("localize", "debugging"))
	assert.Equal(t, "implementation", familyForPhase("reproduce", "implementation"))
	assert.Equal(t, "implementation", familyForPhase("edit", "implementation"))
}
*/
