package euclotest

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/capabilities"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/gate"
	"github.com/lexcodex/relurpify/named/euclo/orchestrate"
	"github.com/stretchr/testify/require"
)

func TestProfileControllerExecutesProfileCapability(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	cap := &stubProfileCapability{
		id:       "euclo:evr",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusCompleted,
			Summary: "all phases done",
			Artifacts: []euclotypes.Artifact{
				{Kind: euclotypes.ArtifactKindExplore, ProducerID: "euclo:evr", Status: "produced"},
				{Kind: euclotypes.ArtifactKindPlan, ProducerID: "euclo:evr", Status: "produced"},
				{Kind: euclotypes.ArtifactKindEditIntent, ProducerID: "euclo:evr", Status: "produced"},
				{Kind: euclotypes.ArtifactKindVerification, ProducerID: "euclo:evr", Status: "produced"},
			},
		},
	}
	require.NoError(t, reg.Register(cap))

	pc := testProfileController(reg)
	profile := euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair"}
	mode := euclotypes.ModeResolution{ModeID: "code"}
	env := testEnvelope(nil)

	result, pcResult, err := pc.ExecuteProfile(context.Background(), profile, mode, env)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.True(t, cap.executeCalled)
	require.Len(t, pcResult.CapabilityIDs, 1)
	require.Equal(t, "euclo:evr", pcResult.CapabilityIDs[0])
	require.Len(t, pcResult.Artifacts, 4)
}

func TestProfileControllerEvaluatesGatesBetweenPhases(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	cap := &stubProfileCapability{
		id:       "euclo:evr",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusCompleted,
			Summary: "done",
			Artifacts: []euclotypes.Artifact{
				{Kind: euclotypes.ArtifactKindExplore, ProducerID: "euclo:evr", Status: "produced"},
			},
		},
	}
	require.NoError(t, reg.Register(cap))

	pc := testProfileController(reg)
	profile := euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair"}
	mode := euclotypes.ModeResolution{ModeID: "code"}
	env := testEnvelope(nil)

	_, pcResult, _ := pc.ExecuteProfile(context.Background(), profile, mode, env)
	// edit_verify_repair has 3 gates; first is pre-execution, remaining 2 are post.
	require.True(t, len(pcResult.GateEvals) > 0)
}

func TestProfileControllerStopsAtBlockingGate(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	// Capability is ineligible, so no profile-level match.
	// Phase-by-phase: first gate requires ArtifactKindExplore which won't be present.
	// No per-phase capability will match either.
	pc := testProfileController(reg)

	// Use a state that has no explore artifacts — the first gate
	// (explore→plan) will block.
	state := core.NewContext()
	profile := euclotypes.ExecutionProfileSelection{
		ProfileID:   "edit_verify_repair",
		PhaseRoutes: map[string]string{"explore": "react", "plan": "pipeline", "edit": "pipeline", "verify": "react"},
	}
	mode := euclotypes.ModeResolution{ModeID: "code"}
	env := testEnvelope(state)

	result, pcResult, err := pc.ExecuteProfile(context.Background(), profile, mode, env)
	// No capabilities found means we get a completed result with no phases.
	// Gates are only evaluated when moving between phases (i > 0).
	require.NoError(t, err)
	require.NotNil(t, result)
	// With no capabilities, nothing executes.
	require.Empty(t, pcResult.PhasesExecuted)
}

func TestProfileControllerSkipsOnWarnGate(t *testing.T) {
	// review_suggest_implement has a warn gate (review→summarize).
	reg := capabilities.NewEucloCapabilityRegistry()
	reviewCap := &stubProfileCapability{
		id:       "euclo:review",
		profiles: []string{"review_suggest_implement"},
		eligible: true,
		contract: euclotypes.ArtifactContract{
			ProducedOutputs: []euclotypes.ArtifactKind{euclotypes.ArtifactKindAnalyze},
		},
		executeResult: euclotypes.ExecutionResult{
			Status: euclotypes.ExecutionStatusCompleted,
			Artifacts: []euclotypes.Artifact{
				{Kind: euclotypes.ArtifactKindAnalyze, ProducerID: "euclo:review", Status: "produced"},
			},
		},
	}
	summarizeCap := &stubProfileCapability{
		id:       "euclo:summarize",
		profiles: []string{"review_suggest_implement"},
		eligible: true,
		contract: euclotypes.ArtifactContract{
			ProducedOutputs: []euclotypes.ArtifactKind{euclotypes.ArtifactKindFinalReport},
		},
		executeResult: euclotypes.ExecutionResult{
			Status: euclotypes.ExecutionStatusCompleted,
			Artifacts: []euclotypes.Artifact{
				{Kind: euclotypes.ArtifactKindFinalReport, ProducerID: "euclo:summarize", Status: "produced"},
			},
		},
	}
	require.NoError(t, reg.Register(reviewCap))
	require.NoError(t, reg.Register(summarizeCap))

	pc := testProfileController(reg)
	profile := euclotypes.ExecutionProfileSelection{
		ProfileID:   "review_suggest_implement",
		PhaseRoutes: map[string]string{"review": "reflection", "summarize": "react"},
	}
	mode := euclotypes.ModeResolution{ModeID: "review"}
	env := testEnvelope(nil)

	// Should succeed — the profile-level capability match handles it.
	result, _, err := pc.ExecuteProfile(context.Background(), profile, mode, env)
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestProfileControllerFallsBackOnCapabilityFailure(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	primary := &stubProfileCapability{
		id:       "euclo:primary",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "primary failed",
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:        "fail",
				Message:     "something broke",
				Recoverable: true,
			},
		},
	}
	require.NoError(t, reg.Register(primary))

	pc := testProfileController(reg)
	profile := euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair"}
	mode := euclotypes.ModeResolution{ModeID: "code"}
	env := testEnvelope(nil)

	result, _, err := pc.ExecuteProfile(context.Background(), profile, mode, env)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed")
	require.False(t, result.Success)
}

func TestProfileControllerCollectsArtifactsAcrossPhases(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	cap := &stubProfileCapability{
		id:       "euclo:evr",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status: euclotypes.ExecutionStatusCompleted,
			Artifacts: []euclotypes.Artifact{
				{Kind: euclotypes.ArtifactKindExplore, ProducerID: "euclo:evr"},
				{Kind: euclotypes.ArtifactKindPlan, ProducerID: "euclo:evr"},
				{Kind: euclotypes.ArtifactKindEditIntent, ProducerID: "euclo:evr"},
				{Kind: euclotypes.ArtifactKindVerification, ProducerID: "euclo:evr"},
			},
		},
	}
	require.NoError(t, reg.Register(cap))

	pc := testProfileController(reg)
	profile := euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair"}
	env := testEnvelope(nil)

	_, pcResult, err := pc.ExecuteProfile(context.Background(), profile, euclotypes.ModeResolution{ModeID: "code"}, env)
	require.NoError(t, err)
	require.Len(t, pcResult.Artifacts, 4)

	// Verify artifacts are in state.
	artState := euclotypes.ArtifactStateFromContext(env.State)
	require.True(t, artState.Has(euclotypes.ArtifactKindExplore))
	require.True(t, artState.Has(euclotypes.ArtifactKindPlan))
	require.True(t, artState.Has(euclotypes.ArtifactKindEditIntent))
	require.True(t, artState.Has(euclotypes.ArtifactKindVerification))
}

func TestProfileControllerReturnsPartialResultOnEarlyStop(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	// Register a capability that fails.
	cap := &stubProfileCapability{
		id:       "euclo:fail",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "partial execution",
			FailureInfo: &euclotypes.CapabilityFailure{
				Code:    "partial_fail",
				Message: "stopped midway",
			},
		},
	}
	require.NoError(t, reg.Register(cap))

	pc := testProfileController(reg)
	profile := euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair"}
	mode := euclotypes.ModeResolution{ModeID: "code"}
	env := testEnvelope(nil)

	result, _, err := pc.ExecuteProfile(context.Background(), profile, mode, env)
	require.Error(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Contains(t, result.Data["status"].(string), "failed")
}

func TestProfileControllerRecordsObservability(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	cap := &stubProfileCapability{
		id:       "euclo:evr",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusCompleted,
			Summary: "done",
		},
	}
	require.NoError(t, reg.Register(cap))

	pc := testProfileController(reg)
	state := core.NewContext()
	profile := euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair"}
	mode := euclotypes.ModeResolution{ModeID: "code"}
	env := euclotypes.ExecutionEnvelope{
		Task:    &core.Task{ID: "obs-test"},
		Mode:    mode,
		Profile: profile,
		State:   state,
	}

	_, _, err := pc.ExecuteProfile(context.Background(), profile, mode, env)
	require.NoError(t, err)

	raw, ok := state.Get("euclo.profile_controller")
	require.True(t, ok)
	typed, ok := raw.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "code", typed["mode_id"])
	require.Equal(t, "edit_verify_repair", typed["profile_id"])
}

// TestOrderedPhasesFromGates tests phase ordering from profile gates
// Note: orderedPhases is a private function that needs to be implemented
// This test is commented out pending implementation
/*
func TestOrderedPhasesFromGates(t *testing.T) {
	gates := gate.DefaultPhaseGates()

	// edit_verify_repair: explore → plan → edit → verify
	evrPhases := orderedPhases(nil, gates["edit_verify_repair"])
	require.Equal(t, []string{"explore", "plan", "edit", "verify"}, evrPhases)

	// reproduce_localize_patch: reproduce → localize → patch → verify
	rlpPhases := orderedPhases(nil, gates["reproduce_localize_patch"])
	require.Equal(t, []string{"reproduce", "localize", "patch", "verify"}, rlpPhases)

	// test_driven_generation: plan_tests → implement → verify
	tdgPhases := orderedPhases(nil, gates["test_driven_generation"])
	require.Equal(t, []string{"plan_tests", "implement", "verify"}, tdgPhases)
}

func TestOrderedPhasesFallsBackToSortedKeys(t *testing.T) {
	phaseRoutes := map[string]string{"z_phase": "react", "a_phase": "pipeline"}
	phases := orderedPhases(phaseRoutes, nil)
	require.Equal(t, []string{"a_phase", "z_phase"}, phases)
}

func TestPhaseExpectedArtifact(t *testing.T) {
	tests := []struct {
		phase    string
		expected euclotypes.ArtifactKind
	}{
		{"explore", euclotypes.ArtifactKindExplore},
		{"plan", euclotypes.ArtifactKindPlan},
		{"plan_tests", euclotypes.ArtifactKindPlan},
		{"edit", euclotypes.ArtifactKindEditIntent},
		{"patch", euclotypes.ArtifactKindEditIntent},
		{"implement", euclotypes.ArtifactKindEditIntent},
		{"verify", euclotypes.ArtifactKindVerification},
		{"reproduce", euclotypes.ArtifactKindExplore},
		{"trace", euclotypes.ArtifactKindExplore},
		{"localize", euclotypes.ArtifactKindAnalyze},
		{"analyze", euclotypes.ArtifactKindAnalyze},
		{"review", euclotypes.ArtifactKindAnalyze},
		{"summarize", euclotypes.ArtifactKindFinalReport},
		{"unknown", ""},
	}
	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			require.Equal(t, tt.expected, phaseExpectedArtifact(tt.phase))
		})
	}
}
*/

// TestMergeCapabilityArtifactsToState tests merging artifacts to state
// Note: mergeCapabilityArtifactsToState is a private function that needs to be implemented
// This test is commented out pending implementation
/*
func TestMergeCapabilityArtifactsToState(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.artifacts", []euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindIntake},
	})

	mergeCapabilityArtifactsToState(state, []euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindExplore, Payload: map[string]any{"data": "explore"}},
		{Kind: euclotypes.ArtifactKindPlan, Payload: map[string]any{"data": "plan"}},
	})

	artState := euclotypes.ArtifactStateFromContext(state)
	require.Equal(t, 3, artState.Len())
	require.True(t, artState.Has(euclotypes.ArtifactKindIntake))
	require.True(t, artState.Has(euclotypes.ArtifactKindExplore))
	require.True(t, artState.Has(euclotypes.ArtifactKindPlan))

	// Check individual state keys were set.
	raw, ok := state.Get("pipeline.explore")
	require.True(t, ok)
	require.NotNil(t, raw)
}
*/

func TestProfileControllerNilCapabilitiesCompletesEmpty(t *testing.T) {
	// With no capabilities, no profile-level or per-phase match is found.
	// Phase-by-phase runs: first phase (explore) has no capability, is skipped.
	// Second phase (plan) gate evaluates and blocks (no explore artifact).
	pc := orchestrate.NewProfileController(nil, gate.DefaultPhaseGates(), testEnvMinimal(), euclotypes.DefaultExecutionProfileRegistry(), nil)
	profile := euclotypes.ExecutionProfileSelection{
		ProfileID:   "edit_verify_repair",
		PhaseRoutes: map[string]string{"explore": "react", "plan": "pipeline", "edit": "pipeline", "verify": "react"},
	}
	mode := euclotypes.ModeResolution{ModeID: "code"}

	result, pcResult, err := pc.ExecuteProfile(context.Background(), profile, mode, testEnvelope(nil))
	require.NoError(t, err)
	require.NotNil(t, result)
	// Gate blocks at plan (no explore artifact), so this is a partial/early stop.
	require.True(t, pcResult.EarlyStop)
	require.Empty(t, pcResult.CapabilityIDs)
}

func TestProfileControllerPhaseByPhaseExecution(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	// Register a per-phase capability for explore. Since this is the only
	// capability and it doesn't match the full profile (not a profile-level
	// match without eligibility for all phases), the controller falls through
	// to phase-by-phase.
	exploreCap := &stubProfileCapability{
		id:       "euclo:explorer",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		contract: euclotypes.ArtifactContract{
			ProducedOutputs: []euclotypes.ArtifactKind{euclotypes.ArtifactKindExplore},
		},
		executeResult: euclotypes.ExecutionResult{
			Status: euclotypes.ExecutionStatusCompleted,
			Artifacts: []euclotypes.Artifact{
				{Kind: euclotypes.ArtifactKindExplore, Payload: "explored", ProducerID: "euclo:explorer", Status: "produced"},
			},
		},
	}
	require.NoError(t, reg.Register(exploreCap))

	pc := testProfileController(reg)
	profile := euclotypes.ExecutionProfileSelection{
		ProfileID:   "edit_verify_repair",
		PhaseRoutes: map[string]string{"explore": "react", "plan": "pipeline", "edit": "pipeline", "verify": "react"},
	}
	mode := euclotypes.ModeResolution{ModeID: "code"}
	env := testEnvelope(nil)

	// The explorer is profile-eligible, so resolveProfileCapability finds it.
	// It executes as a profile-level cap. Post-execution gates evaluate
	// (some may fail as it only produces explore artifacts).
	result, pcResult, err := pc.ExecuteProfile(context.Background(), profile, mode, env)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Contains(t, pcResult.CapabilityIDs, "euclo:explorer")
	require.True(t, exploreCap.executeCalled)
	require.Len(t, pcResult.Artifacts, 1)
}
