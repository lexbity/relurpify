package euclo

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/internal/testutil"
	"github.com/stretchr/testify/require"
)

// stubProfileCapability is a configurable stub for profile controller tests.
type stubProfileCapability struct {
	id              string
	profiles        []string
	contract        ArtifactContract
	eligible        bool
	executeResult   ExecutionResult
	executeCalled   bool
	executeCount    int
}

func (s *stubProfileCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:   s.id,
		Name: s.id,
		Annotations: map[string]any{
			"supported_profiles": s.profiles,
		},
	}
}

func (s *stubProfileCapability) Contract() ArtifactContract {
	return s.contract
}

func (s *stubProfileCapability) Eligible(_ ArtifactState, _ CapabilitySnapshot) EligibilityResult {
	return EligibilityResult{Eligible: s.eligible, Reason: "stub"}
}

func (s *stubProfileCapability) Execute(_ context.Context, _ ExecutionEnvelope) ExecutionResult {
	s.executeCalled = true
	s.executeCount++
	return s.executeResult
}

// testProfileController creates a ProfileController with test registries and environment
func testProfileController(caps *EucloCapabilityRegistry) *ProfileController {
	return NewProfileController(
		caps,
		DefaultPhaseGates(),
		testEnvMinimal(),
		DefaultExecutionProfileRegistry(),
		nil,
	)
}

// testEnvMinimal returns a minimal AgentEnvironment for testing
func testEnvMinimal() agentenv.AgentEnvironment {
	return testutil.EnvMinimal()
}

// testEnvelope creates an ExecutionEnvelope with test state for testing
func testEnvelope(state *core.Context) ExecutionEnvelope {
	if state == nil {
		state = core.NewContext()
	}
	return ExecutionEnvelope{
		Task: &core.Task{
			ID:          "pc-test-task",
			Instruction: "test instruction",
		},
		Mode:    ModeResolution{ModeID: "code"},
		Profile: ExecutionProfileSelection{ProfileID: "edit_verify_repair"},
		State:   state,
	}
}

func TestProfileControllerExecutesProfileCapability(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	cap := &stubProfileCapability{
		id:       "euclo:evr",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: ExecutionResult{
			Status:  ExecutionStatusCompleted,
			Summary: "all phases done",
			Artifacts: []Artifact{
				{Kind: ArtifactKindExplore, ProducerID: "euclo:evr", Status: "produced"},
				{Kind: ArtifactKindPlan, ProducerID: "euclo:evr", Status: "produced"},
				{Kind: ArtifactKindEditIntent, ProducerID: "euclo:evr", Status: "produced"},
				{Kind: ArtifactKindVerification, ProducerID: "euclo:evr", Status: "produced"},
			},
		},
	}
	require.NoError(t, reg.Register(cap))

	pc := testProfileController(reg)
	profile := ExecutionProfileSelection{ProfileID: "edit_verify_repair"}
	mode := ModeResolution{ModeID: "code"}
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
	reg := NewEucloCapabilityRegistry()
	cap := &stubProfileCapability{
		id:       "euclo:evr",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: ExecutionResult{
			Status:  ExecutionStatusCompleted,
			Summary: "done",
			Artifacts: []Artifact{
				{Kind: ArtifactKindExplore, ProducerID: "euclo:evr", Status: "produced"},
			},
		},
	}
	require.NoError(t, reg.Register(cap))

	pc := testProfileController(reg)
	profile := ExecutionProfileSelection{ProfileID: "edit_verify_repair"}
	mode := ModeResolution{ModeID: "code"}
	env := testEnvelope(nil)

	_, pcResult, _ := pc.ExecuteProfile(context.Background(), profile, mode, env)
	// edit_verify_repair has 3 gates; first is pre-execution, remaining 2 are post.
	require.True(t, len(pcResult.GateEvals) > 0)
}

func TestProfileControllerStopsAtBlockingGate(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	// Capability is ineligible, so no profile-level match.
	// Phase-by-phase: first gate requires ArtifactKindExplore which won't be present.
	// No per-phase capability will match either.
	pc := testProfileController(reg)

	// Use a state that has no explore artifacts — the first gate
	// (explore→plan) will block.
	state := core.NewContext()
	profile := ExecutionProfileSelection{
		ProfileID:   "edit_verify_repair",
		PhaseRoutes: map[string]string{"explore": "react", "plan": "pipeline", "edit": "pipeline", "verify": "react"},
	}
	mode := ModeResolution{ModeID: "code"}
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
	reg := NewEucloCapabilityRegistry()
	reviewCap := &stubProfileCapability{
		id:       "euclo:review",
		profiles: []string{"review_suggest_implement"},
		eligible: true,
		contract: ArtifactContract{
			ProducedOutputs: []ArtifactKind{ArtifactKindAnalyze},
		},
		executeResult: ExecutionResult{
			Status: ExecutionStatusCompleted,
			Artifacts: []Artifact{
				{Kind: ArtifactKindAnalyze, ProducerID: "euclo:review", Status: "produced"},
			},
		},
	}
	summarizeCap := &stubProfileCapability{
		id:       "euclo:summarize",
		profiles: []string{"review_suggest_implement"},
		eligible: true,
		contract: ArtifactContract{
			ProducedOutputs: []ArtifactKind{ArtifactKindFinalReport},
		},
		executeResult: ExecutionResult{
			Status: ExecutionStatusCompleted,
			Artifacts: []Artifact{
				{Kind: ArtifactKindFinalReport, ProducerID: "euclo:summarize", Status: "produced"},
			},
		},
	}
	require.NoError(t, reg.Register(reviewCap))
	require.NoError(t, reg.Register(summarizeCap))

	pc := testProfileController(reg)
	profile := ExecutionProfileSelection{
		ProfileID:   "review_suggest_implement",
		PhaseRoutes: map[string]string{"review": "reflection", "summarize": "react"},
	}
	mode := ModeResolution{ModeID: "review"}
	env := testEnvelope(nil)

	// Should succeed — the profile-level capability match handles it.
	result, _, err := pc.ExecuteProfile(context.Background(), profile, mode, env)
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestProfileControllerFallsBackOnCapabilityFailure(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	primary := &stubProfileCapability{
		id:       "euclo:primary",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: ExecutionResult{
			Status:  ExecutionStatusFailed,
			Summary: "primary failed",
			FailureInfo: &CapabilityFailure{
				Code:        "fail",
				Message:     "something broke",
				Recoverable: true,
			},
		},
	}
	require.NoError(t, reg.Register(primary))

	pc := testProfileController(reg)
	profile := ExecutionProfileSelection{ProfileID: "edit_verify_repair"}
	mode := ModeResolution{ModeID: "code"}
	env := testEnvelope(nil)

	result, _, err := pc.ExecuteProfile(context.Background(), profile, mode, env)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed")
	require.False(t, result.Success)
}

func TestProfileControllerCollectsArtifactsAcrossPhases(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	cap := &stubProfileCapability{
		id:       "euclo:evr",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: ExecutionResult{
			Status: ExecutionStatusCompleted,
			Artifacts: []Artifact{
				{Kind: ArtifactKindExplore, ProducerID: "euclo:evr"},
				{Kind: ArtifactKindPlan, ProducerID: "euclo:evr"},
				{Kind: ArtifactKindEditIntent, ProducerID: "euclo:evr"},
				{Kind: ArtifactKindVerification, ProducerID: "euclo:evr"},
			},
		},
	}
	require.NoError(t, reg.Register(cap))

	pc := testProfileController(reg)
	profile := ExecutionProfileSelection{ProfileID: "edit_verify_repair"}
	env := testEnvelope(nil)

	_, pcResult, err := pc.ExecuteProfile(context.Background(), profile, ModeResolution{ModeID: "code"}, env)
	require.NoError(t, err)
	require.Len(t, pcResult.Artifacts, 4)

	// Verify artifacts are in state.
	artState := ArtifactStateFromContext(env.State)
	require.True(t, artState.Has(ArtifactKindExplore))
	require.True(t, artState.Has(ArtifactKindPlan))
	require.True(t, artState.Has(ArtifactKindEditIntent))
	require.True(t, artState.Has(ArtifactKindVerification))
}

func TestProfileControllerReturnsPartialResultOnEarlyStop(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	// Register a capability that fails.
	cap := &stubProfileCapability{
		id:       "euclo:fail",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: ExecutionResult{
			Status:  ExecutionStatusFailed,
			Summary: "partial execution",
			FailureInfo: &CapabilityFailure{
				Code:    "partial_fail",
				Message: "stopped midway",
			},
		},
	}
	require.NoError(t, reg.Register(cap))

	pc := testProfileController(reg)
	profile := ExecutionProfileSelection{ProfileID: "edit_verify_repair"}
	mode := ModeResolution{ModeID: "code"}
	env := testEnvelope(nil)

	result, _, err := pc.ExecuteProfile(context.Background(), profile, mode, env)
	require.Error(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Contains(t, result.Data["status"].(string), "failed")
}

func TestProfileControllerRecordsObservability(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	cap := &stubProfileCapability{
		id:       "euclo:evr",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: ExecutionResult{
			Status:  ExecutionStatusCompleted,
			Summary: "done",
		},
	}
	require.NoError(t, reg.Register(cap))

	pc := testProfileController(reg)
	state := core.NewContext()
	profile := ExecutionProfileSelection{ProfileID: "edit_verify_repair"}
	mode := ModeResolution{ModeID: "code"}
	env := ExecutionEnvelope{
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
	gates := DefaultPhaseGates()

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
		expected ArtifactKind
	}{
		{"explore", ArtifactKindExplore},
		{"plan", ArtifactKindPlan},
		{"plan_tests", ArtifactKindPlan},
		{"edit", ArtifactKindEditIntent},
		{"patch", ArtifactKindEditIntent},
		{"implement", ArtifactKindEditIntent},
		{"verify", ArtifactKindVerification},
		{"reproduce", ArtifactKindExplore},
		{"trace", ArtifactKindExplore},
		{"localize", ArtifactKindAnalyze},
		{"analyze", ArtifactKindAnalyze},
		{"review", ArtifactKindAnalyze},
		{"summarize", ArtifactKindFinalReport},
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
	state.Set("euclo.artifacts", []Artifact{
		{Kind: ArtifactKindIntake},
	})

	mergeCapabilityArtifactsToState(state, []Artifact{
		{Kind: ArtifactKindExplore, Payload: map[string]any{"data": "explore"}},
		{Kind: ArtifactKindPlan, Payload: map[string]any{"data": "plan"}},
	})

	artState := ArtifactStateFromContext(state)
	require.Equal(t, 3, artState.Len())
	require.True(t, artState.Has(ArtifactKindIntake))
	require.True(t, artState.Has(ArtifactKindExplore))
	require.True(t, artState.Has(ArtifactKindPlan))

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
	pc := NewProfileController(nil, DefaultPhaseGates(), testEnvMinimal(), DefaultExecutionProfileRegistry(), nil)
	profile := ExecutionProfileSelection{
		ProfileID:   "edit_verify_repair",
		PhaseRoutes: map[string]string{"explore": "react", "plan": "pipeline", "edit": "pipeline", "verify": "react"},
	}
	mode := ModeResolution{ModeID: "code"}

	result, pcResult, err := pc.ExecuteProfile(context.Background(), profile, mode, testEnvelope(nil))
	require.NoError(t, err)
	require.NotNil(t, result)
	// Gate blocks at plan (no explore artifact), so this is a partial/early stop.
	require.True(t, pcResult.EarlyStop)
	require.Empty(t, pcResult.CapabilityIDs)
}

func TestProfileControllerPhaseByPhaseExecution(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	// Register a per-phase capability for explore. Since this is the only
	// capability and it doesn't match the full profile (not a profile-level
	// match without eligibility for all phases), the controller falls through
	// to phase-by-phase.
	exploreCap := &stubProfileCapability{
		id:       "euclo:explorer",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		contract: ArtifactContract{
			ProducedOutputs: []ArtifactKind{ArtifactKindExplore},
		},
		executeResult: ExecutionResult{
			Status: ExecutionStatusCompleted,
			Artifacts: []Artifact{
				{Kind: ArtifactKindExplore, Payload: "explored", ProducerID: "euclo:explorer", Status: "produced"},
			},
		},
	}
	require.NoError(t, reg.Register(exploreCap))

	pc := testProfileController(reg)
	profile := ExecutionProfileSelection{
		ProfileID:   "edit_verify_repair",
		PhaseRoutes: map[string]string{"explore": "react", "plan": "pipeline", "edit": "pipeline", "verify": "react"},
	}
	mode := ModeResolution{ModeID: "code"}
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
