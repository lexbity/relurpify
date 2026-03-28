package euclotest

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/capabilities"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/gate"
	"github.com/lexcodex/relurpify/named/euclo/orchestrate"
	"github.com/lexcodex/relurpify/testutil/euclotestutil"
	"github.com/stretchr/testify/require"
)

func TestDefaultCapabilityRegistryRegistersPhaseOneThroughEightCapabilities(t *testing.T) {
	env := testEnv(t)
	reg := capabilities.NewDefaultCapabilityRegistry(env)
	ids := make([]string, 0, len(reg.List()))
	for _, cap := range reg.List() {
		ids = append(ids, cap.Descriptor().ID)
	}

	require.Contains(t, ids, "euclo:edit_verify_repair")
	require.Contains(t, ids, "euclo:debug.investigate_regression")
	require.Contains(t, ids, "euclo:design.alternatives")
	require.Contains(t, ids, "euclo:trace.analyze")
	require.Contains(t, ids, "euclo:review.findings")
	require.Contains(t, ids, "euclo:refactor.api_compatible")
	require.Contains(t, ids, "euclo:artifact.diff_summary")
	require.Contains(t, ids, "euclo:migration.execute")
	require.Len(t, ids, 18)
}

func TestProfileRoutingIncludesNewPhaseCapabilities(t *testing.T) {
	env := testEnv(t)
	reg := capabilities.NewDefaultCapabilityRegistry(env)

	planStage := capabilityIDs(reg.ForProfile("plan_stage_execute"))
	require.Contains(t, planStage, "euclo:design.alternatives")
	require.Contains(t, planStage, "euclo:execution_profile.select")
	require.Contains(t, planStage, "euclo:refactor.api_compatible")
	require.Contains(t, planStage, "euclo:migration.execute")

	debugProfile := capabilityIDs(reg.ForProfile("reproduce_localize_patch"))
	require.Contains(t, debugProfile, "euclo:debug.investigate_regression")
	require.Contains(t, debugProfile, "euclo:artifact.trace_to_root_cause")
	require.Contains(t, debugProfile, "euclo:artifact.verification_summary")

	reviewProfile := capabilityIDs(reg.ForProfile("review_suggest_implement"))
	require.Contains(t, reviewProfile, "euclo:review.findings")
	require.Contains(t, reviewProfile, "euclo:review.compatibility")
	require.Contains(t, reviewProfile, "euclo:review.implement_if_safe")
}

func TestPhaseGatesAcceptNewArtifactKinds(t *testing.T) {
	gates := gate.DefaultPhaseGates()

	migrationEval := gate.EvaluateGate(gates["plan_stage_execute"][0], "planning", euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindMigrationPlan, Payload: map[string]any{"steps": []any{map[string]any{"id": "m1"}}}},
	}))
	require.True(t, migrationEval.Passed)

	traceEval := gate.EvaluateGate(gates["trace_execute_analyze"][0], "debug", euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindTrace, Payload: map[string]any{"frames": []any{map[string]any{"function": "Run"}}}},
	}))
	require.True(t, traceEval.Passed)

	reviewEval := gate.EvaluateGate(gates["review_suggest_implement"][0], "review", euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindReviewFindings, Payload: map[string]any{"findings": []any{map[string]any{"severity": "warning"}}}},
	}))
	require.True(t, reviewEval.Passed)

	regressionEval := gate.EvaluateGate(gates["reproduce_localize_patch"][1], "debug", euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindRegressionAnalysis, Payload: map[string]any{"summary": "narrowed suspect window"}},
	}))
	require.True(t, regressionEval.Passed)
}

func TestRecoveryControllerUsesFallbackChainsAndPreferredProfiles(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	require.NoError(t, reg.Register(&stubProfileCapability{
		id:       "euclo:first_fallback",
		profiles: []string{"edit_verify_repair"},
		eligible: false,
	}))
	second := &stubProfileCapability{
		id:       "euclo:second_fallback",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusCompleted,
			Summary: "second fallback succeeded",
		},
	}
	require.NoError(t, reg.Register(second))
	preferredProfileCap := &stubProfileCapability{
		id:       "euclo:preferred_profile_cap",
		profiles: []string{"plan_stage_execute"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusCompleted,
			Summary: "preferred profile succeeded",
		},
	}
	require.NoError(t, reg.Register(preferredProfileCap))

	profiles := euclotypes.NewExecutionProfileRegistry()
	require.NoError(t, profiles.Register(euclotypes.ExecutionProfileDescriptor{
		ProfileID:            "custom_primary",
		SupportedModes:       []string{"code"},
		PhaseRoutes:          map[string]string{"explore": "react"},
		FallbackProfiles:     []string{"reproduce_localize_patch", "plan_stage_execute"},
		VerificationRequired: true,
	}))
	require.NoError(t, profiles.Register(euclotypes.ExecutionProfileDescriptor{
		ProfileID:      "plan_stage_execute",
		SupportedModes: []string{"code"},
		PhaseRoutes:    map[string]string{"plan": "planner"},
	}))
	require.NoError(t, profiles.Register(euclotypes.ExecutionProfileDescriptor{
		ProfileID:      "reproduce_localize_patch",
		SupportedModes: []string{"code"},
		PhaseRoutes:    map[string]string{"reproduce": "react"},
	}))

	env := testEnv(t)
	require.NoError(t, env.Registry.Register(profileReadTool{}))
	require.NoError(t, env.Registry.Register(testutil.FileWriteTool{}))
	require.NoError(t, env.Registry.Register(profileTestRunnerTool{}))
	rc := orchestrate.NewRecoveryController(orchestrate.AdaptCapabilityRegistry(reg), profiles, euclotypes.DefaultModeRegistry(), env)

	state := core.NewContext()
	state.Set("euclo.artifacts", []euclotypes.Artifact{{Kind: euclotypes.ArtifactKindIntake, Payload: map[string]any{"instruction": "debug this"}}})
	envelope := testEnvelope(state)
	envelope.Registry = env.Registry
	envelope.Memory = env.Memory
	envelope.Environment = env

	fallbackRecovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
		SuggestedCapability: "euclo:first_fallback",
		Context:             map[string]any{"fallback_capabilities": []string{"euclo:first_fallback", "euclo:second_fallback"}},
	}, euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}, envelope, orchestrate.NewRecoveryStack())
	require.Equal(t, euclotypes.ExecutionStatusCompleted, fallbackRecovered.Status)
	require.True(t, second.executeCalled)

	profileEnvelope := envelope
	profileEnvelope.Profile = euclotypes.ExecutionProfileSelection{ProfileID: "custom_primary"}
	profileRecovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy: euclotypes.RecoveryStrategyProfileEscalation,
		Context:  map[string]any{"preferred_profile": "plan_stage_execute"},
	}, euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}, profileEnvelope, orchestrate.NewRecoveryStack())
	require.Equal(t, euclotypes.ExecutionStatusCompleted, profileRecovered.Status)
	require.True(t, preferredProfileCap.executeCalled)
}

func capabilityIDs(caps []euclotypes.EucloCodingCapability) []string {
	out := make([]string, 0, len(caps))
	for _, cap := range caps {
		out = append(out, cap.Descriptor().ID)
	}
	return out
}
