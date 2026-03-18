package euclotest

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/capabilities"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/gate"
	"github.com/lexcodex/relurpify/named/euclo/orchestrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecoveryStackDefaults(t *testing.T) {
	stack := orchestrate.NewRecoveryStack()
	require.NotNil(t, stack)
	assert.Equal(t, 3, stack.MaxDepth)
	assert.False(t, stack.Exhausted)
	assert.Empty(t, stack.Attempts)
	assert.True(t, stack.CanAttempt())
}

func TestRecoveryStackCanAttempt(t *testing.T) {
	stack := orchestrate.NewRecoveryStack()
	// Fill to max depth.
	for i := 0; i < 3; i++ {
		assert.True(t, stack.CanAttempt())
		stack.Record(orchestrate.RecoveryAttempt{Level: orchestrate.RecoveryLevelParadigm, Success: false})
	}
	assert.False(t, stack.CanAttempt())
	assert.True(t, stack.Exhausted)
	assert.Len(t, stack.Attempts, 3)
}

func TestRecoveryStackNilSafe(t *testing.T) {
	var stack *orchestrate.RecoveryStack
	assert.False(t, stack.CanAttempt())
	stack.Record(orchestrate.RecoveryAttempt{}) // should not panic
}

func TestRecoveryStackExhaustedPreventsAttempt(t *testing.T) {
	stack := orchestrate.NewRecoveryStack()
	stack.Exhausted = true
	assert.False(t, stack.CanAttempt())
}

func TestRecoveryTraceArtifact(t *testing.T) {
	stack := orchestrate.NewRecoveryStack()
	stack.Record(orchestrate.RecoveryAttempt{
		Level:    orchestrate.RecoveryLevelCapability,
		Strategy: euclotypes.RecoveryStrategyCapabilityFallback,
		From:     "cap_a",
		To:       "cap_b",
		Reason:   "fallback",
		Success:  true,
	})
	stack.Record(orchestrate.RecoveryAttempt{
		Level:    orchestrate.RecoveryLevelProfile,
		Strategy: euclotypes.RecoveryStrategyProfileEscalation,
		From:     "profile_a",
		To:       "profile_b",
		Reason:   "escalation",
		Success:  false,
	})

	art := orchestrate.RecoveryTraceArtifact(stack, "euclo:test")
	assert.Equal(t, euclotypes.ArtifactKindRecoveryTrace, art.Kind)
	assert.Equal(t, "euclo:test", art.ProducerID)
	assert.Equal(t, "produced", art.Status)
	assert.Contains(t, art.Summary, "2 recovery attempts")

	payload, ok := art.Payload.(map[string]any)
	require.True(t, ok)
	attempts, ok := payload["attempts"].([]map[string]any)
	require.True(t, ok)
	assert.Len(t, attempts, 2)
	assert.Equal(t, "capability", attempts[0]["level"])
	assert.Equal(t, "capability_fallback", attempts[0]["strategy"])
	assert.Equal(t, true, attempts[0]["success"])
	assert.Equal(t, "profile", attempts[1]["level"])
	assert.Equal(t, "profile_escalation", attempts[1]["strategy"])
	assert.Equal(t, false, attempts[1]["success"])
}

func TestRecoveryControllerNilSafe(t *testing.T) {
	var rc *orchestrate.RecoveryController
	stack := orchestrate.NewRecoveryStack()
	result := euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}
	env := testEnvelope(nil)

	recovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy: euclotypes.RecoveryStrategyCapabilityFallback,
	}, result, env, stack)

	assert.Equal(t, euclotypes.ExecutionStatusFailed, recovered.Status)
	assert.Empty(t, stack.Attempts)
}

func TestRecoveryControllerExhaustedStack(t *testing.T) {
	env := testEnv(t)
	reg := capabilities.NewEucloCapabilityRegistry()
	rc := orchestrate.NewRecoveryController(
		orchestrate.AdaptCapabilityRegistry(reg),
		euclotypes.DefaultExecutionProfileRegistry(),
		euclotypes.DefaultModeRegistry(),
		env,
	)
	stack := orchestrate.NewRecoveryStack()
	stack.Exhausted = true

	result := euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}
	recovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy: euclotypes.RecoveryStrategyCapabilityFallback,
	}, result, testEnvelope(nil), stack)

	assert.Equal(t, euclotypes.ExecutionStatusFailed, recovered.Status)
}

// --- Paradigm Switch Tests ---

func TestRecoveryParadigmSwitchSuccess(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	stub := &stubProfileCapability{
		id:       "euclo:test_cap",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusCompleted,
			Summary: "paradigm switch succeeded",
			Artifacts: []euclotypes.Artifact{
				{Kind: euclotypes.ArtifactKindEditIntent, ProducerID: "euclo:test_cap"},
			},
		},
	}
	_ = reg.Register(stub)

	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(orchestrate.AdaptCapabilityRegistry(reg), euclotypes.DefaultExecutionProfileRegistry(), euclotypes.DefaultModeRegistry(), env)
	stack := orchestrate.NewRecoveryStack()

	failedResult := euclotypes.ExecutionResult{
		Status: euclotypes.ExecutionStatusFailed,
		FailureInfo: &euclotypes.CapabilityFailure{
			ParadigmUsed: "react",
		},
		Artifacts: []euclotypes.Artifact{
			{ProducerID: "euclo:test_cap"},
		},
	}

	recovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy:          euclotypes.RecoveryStrategyParadigmSwitch,
		SuggestedParadigm: "planner",
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, euclotypes.ExecutionStatusCompleted, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.True(t, stack.Attempts[0].Success)
	assert.Equal(t, orchestrate.RecoveryLevelParadigm, stack.Attempts[0].Level)
	assert.Equal(t, "react", stack.Attempts[0].From)
	assert.Equal(t, "planner", stack.Attempts[0].To)
	assert.True(t, stub.executeCalled)
}

func TestRecoveryParadigmSwitchNoSuggestedParadigm(t *testing.T) {
	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(orchestrate.AdaptCapabilityRegistry(capabilities.NewEucloCapabilityRegistry()), nil, nil, env)
	stack := orchestrate.NewRecoveryStack()

	failedResult := euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}
	recovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy: euclotypes.RecoveryStrategyParadigmSwitch,
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, euclotypes.ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
}

func TestRecoveryParadigmSwitchCapabilityNotFound(t *testing.T) {
	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(orchestrate.AdaptCapabilityRegistry(capabilities.NewEucloCapabilityRegistry()), nil, nil, env)
	stack := orchestrate.NewRecoveryStack()

	failedResult := euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusFailed,
		Artifacts: []euclotypes.Artifact{{ProducerID: "euclo:nonexistent"}},
	}
	recovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy:          euclotypes.RecoveryStrategyParadigmSwitch,
		SuggestedParadigm: "planner",
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, euclotypes.ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
	assert.Contains(t, stack.Attempts[0].Reason, "not found")
}

// --- Capability Fallback Tests ---

func TestRecoveryCapabilityFallbackSuccess(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	fallbackCap := &stubProfileCapability{
		id:       "euclo:fallback_cap",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusCompleted,
			Summary: "fallback succeeded",
			Artifacts: []euclotypes.Artifact{
				{Kind: euclotypes.ArtifactKindEditIntent, ProducerID: "euclo:fallback_cap"},
			},
		},
	}
	_ = reg.Register(fallbackCap)

	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(orchestrate.AdaptCapabilityRegistry(reg), nil, nil, env)
	stack := orchestrate.NewRecoveryStack()

	failedResult := euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusFailed,
		Artifacts: []euclotypes.Artifact{{ProducerID: "euclo:original"}},
	}

	recovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
		SuggestedCapability: "euclo:fallback_cap",
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, euclotypes.ExecutionStatusCompleted, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.True(t, stack.Attempts[0].Success)
	assert.Equal(t, orchestrate.RecoveryLevelCapability, stack.Attempts[0].Level)
	assert.True(t, fallbackCap.executeCalled)
}

func TestRecoveryCapabilityFallbackNotEligible(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	_ = reg.Register(&stubProfileCapability{
		id:       "euclo:ineligible",
		profiles: []string{"edit_verify_repair"},
		eligible: false,
	})

	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(orchestrate.AdaptCapabilityRegistry(reg), nil, nil, env)
	stack := orchestrate.NewRecoveryStack()

	failedResult := euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}
	recovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
		SuggestedCapability: "euclo:ineligible",
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, euclotypes.ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
	assert.Contains(t, stack.Attempts[0].Reason, "not eligible")
}

func TestRecoveryCapabilityFallbackNotFound(t *testing.T) {
	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(orchestrate.AdaptCapabilityRegistry(capabilities.NewEucloCapabilityRegistry()), nil, nil, env)
	stack := orchestrate.NewRecoveryStack()

	failedResult := euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}
	recovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
		SuggestedCapability: "euclo:nonexistent",
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, euclotypes.ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
	assert.Contains(t, stack.Attempts[0].Reason, "not found")
}

func TestRecoveryCapabilityFallbackNoSuggestion(t *testing.T) {
	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(orchestrate.AdaptCapabilityRegistry(capabilities.NewEucloCapabilityRegistry()), nil, nil, env)
	stack := orchestrate.NewRecoveryStack()

	failedResult := euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}
	recovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy: euclotypes.RecoveryStrategyCapabilityFallback,
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, euclotypes.ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
}

// --- Profile Escalation Tests ---

func TestRecoveryProfileEscalationSuccess(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	// Register a capability for the fallback profile.
	fallbackCap := &stubProfileCapability{
		id:       "euclo:fallback_profile_cap",
		profiles: []string{"reproduce_localize_patch"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusCompleted,
			Summary: "profile escalation succeeded",
		},
	}
	_ = reg.Register(fallbackCap)

	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(orchestrate.AdaptCapabilityRegistry(reg), euclotypes.DefaultExecutionProfileRegistry(), euclotypes.DefaultModeRegistry(), env)
	stack := orchestrate.NewRecoveryStack()

	failedResult := euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}
	envelope := testEnvelope(nil)
	envelope.Profile = euclotypes.ExecutionProfileSelection{
		ProfileID: "edit_verify_repair",
	}

	recovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy: euclotypes.RecoveryStrategyProfileEscalation,
	}, failedResult, envelope, stack)

	assert.Equal(t, euclotypes.ExecutionStatusCompleted, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.True(t, stack.Attempts[0].Success)
	assert.Equal(t, orchestrate.RecoveryLevelProfile, stack.Attempts[0].Level)
	assert.Equal(t, "edit_verify_repair", stack.Attempts[0].From)
	assert.Equal(t, "reproduce_localize_patch", stack.Attempts[0].To)
	assert.True(t, fallbackCap.executeCalled)
}

func TestRecoveryProfileEscalationNoFallbacks(t *testing.T) {
	// Create a profile registry with a profile that has no fallbacks.
	profiles := euclotypes.NewExecutionProfileRegistry()
	_ = profiles.Register(euclotypes.ExecutionProfileDescriptor{
		ProfileID:      "no_fallback",
		SupportedModes: []string{"code"},
		PhaseRoutes:    map[string]string{"explore": "react"},
	})

	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(orchestrate.AdaptCapabilityRegistry(capabilities.NewEucloCapabilityRegistry()), profiles, nil, env)
	stack := orchestrate.NewRecoveryStack()

	failedResult := euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}
	envelope := testEnvelope(nil)
	envelope.Profile = euclotypes.ExecutionProfileSelection{ProfileID: "no_fallback"}

	recovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy: euclotypes.RecoveryStrategyProfileEscalation,
	}, failedResult, envelope, stack)

	assert.Equal(t, euclotypes.ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
	assert.Contains(t, stack.Attempts[0].Reason, "no fallback profiles")
}

func TestRecoveryProfileEscalationNoEligibleCapability(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	// Capability for fallback profile but not eligible.
	_ = reg.Register(&stubProfileCapability{
		id:       "euclo:ineligible_for_fallback",
		profiles: []string{"reproduce_localize_patch"},
		eligible: false,
	})

	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(orchestrate.AdaptCapabilityRegistry(reg), euclotypes.DefaultExecutionProfileRegistry(), nil, env)
	stack := orchestrate.NewRecoveryStack()

	failedResult := euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}
	envelope := testEnvelope(nil)
	envelope.Profile = euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair"}

	recovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy: euclotypes.RecoveryStrategyProfileEscalation,
	}, failedResult, envelope, stack)

	assert.Equal(t, euclotypes.ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
	assert.Contains(t, stack.Attempts[0].Reason, "no eligible capability")
}

func TestRecoveryProfileEscalationNoProfileRegistry(t *testing.T) {
	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(orchestrate.AdaptCapabilityRegistry(capabilities.NewEucloCapabilityRegistry()), nil, nil, env)
	stack := orchestrate.NewRecoveryStack()

	failedResult := euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}
	recovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy: euclotypes.RecoveryStrategyProfileEscalation,
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, euclotypes.ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
	assert.Contains(t, stack.Attempts[0].Reason, "registry unavailable")
}

// --- Mode Escalation Tests ---

func TestRecoveryModeEscalationRecommendation(t *testing.T) {
	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(nil, nil, euclotypes.DefaultModeRegistry(), env)
	stack := orchestrate.NewRecoveryStack()

	failedResult := euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed, Summary: "original failure"}
	recovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy: euclotypes.RecoveryStrategyModeEscalation,
	}, failedResult, testEnvelope(nil), stack)

	// Mode escalation never succeeds — it returns a recommendation.
	assert.Equal(t, euclotypes.ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
	assert.Equal(t, orchestrate.RecoveryLevelMode, stack.Attempts[0].Level)

	// The recovery hint should be enriched with escalation status.
	require.NotNil(t, recovered.RecoveryHint)
	assert.Equal(t, euclotypes.RecoveryStrategyModeEscalation, recovered.RecoveryHint.Strategy)
	require.NotNil(t, recovered.RecoveryHint.Context)
	assert.Equal(t, "recommended", recovered.RecoveryHint.Context["escalation_status"])
	assert.Equal(t, true, recovered.RecoveryHint.Context["requires_approval"])
}

// --- Max Depth / Stack Tracking ---

func TestRecoveryMaxDepthEnforced(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	// Register a capability that always fails.
	_ = reg.Register(&stubProfileCapability{
		id:       "euclo:always_fails",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status: euclotypes.ExecutionStatusFailed,
			RecoveryHint: &euclotypes.RecoveryHint{
				Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
				SuggestedCapability: "euclo:always_fails",
			},
		},
	})

	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(orchestrate.AdaptCapabilityRegistry(reg), nil, nil, env)
	stack := orchestrate.NewRecoveryStack()

	failedResult := euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}

	// Attempt recovery 3 times (max depth).
	for i := 0; i < 3; i++ {
		rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
			Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
			SuggestedCapability: "euclo:always_fails",
		}, failedResult, testEnvelope(nil), stack)
	}

	assert.True(t, stack.Exhausted)
	assert.Len(t, stack.Attempts, 3)

	// Fourth attempt should be refused.
	recovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
		SuggestedCapability: "euclo:always_fails",
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, euclotypes.ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 3) // no new attempt recorded
}

func TestRecoveryStackTracksMultipleLevels(t *testing.T) {
	stack := orchestrate.NewRecoveryStack()
	stack.Record(orchestrate.RecoveryAttempt{Level: orchestrate.RecoveryLevelParadigm, From: "react", To: "planner", Success: false})
	stack.Record(orchestrate.RecoveryAttempt{Level: orchestrate.RecoveryLevelCapability, From: "cap_a", To: "cap_b", Success: true})
	stack.Record(orchestrate.RecoveryAttempt{Level: orchestrate.RecoveryLevelProfile, From: "p1", To: "p2", Success: false})

	assert.Len(t, stack.Attempts, 3)
	assert.True(t, stack.Exhausted)

	assert.Equal(t, orchestrate.RecoveryLevelParadigm, stack.Attempts[0].Level)
	assert.Equal(t, orchestrate.RecoveryLevelCapability, stack.Attempts[1].Level)
	assert.Equal(t, orchestrate.RecoveryLevelProfile, stack.Attempts[2].Level)
}

func TestRecoveryUnknownStrategy(t *testing.T) {
	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(nil, nil, nil, env)
	stack := orchestrate.NewRecoveryStack()

	failedResult := euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusFailed}
	recovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy: "unknown_strategy",
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, euclotypes.ExecutionStatusFailed, recovered.Status)
	assert.Empty(t, stack.Attempts) // unknown strategy records nothing
}

// --- Integration with ProfileController ---

func TestProfileControllerRecoveryOnProfileLevelFailure(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()

	// Primary capability fails with recovery hint.
	primary := &stubProfileCapability{
		id:       "euclo:primary",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "primary failed",
			RecoveryHint: &euclotypes.RecoveryHint{
				Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
				SuggestedCapability: "euclo:recovery_cap",
			},
			Artifacts: []euclotypes.Artifact{{ProducerID: "euclo:primary"}},
		},
	}
	// Recovery capability succeeds.
	recoveryCap := &stubProfileCapability{
		id:       "euclo:recovery_cap",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusCompleted,
			Summary: "recovery succeeded",
			Artifacts: []euclotypes.Artifact{
				{Kind: euclotypes.ArtifactKindEditIntent, ProducerID: "euclo:recovery_cap"},
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
	assert.Equal(t, 1, pcResult.RecoveryAttempts)

	// Check recovery trace artifact was recorded.
	hasRecoveryTrace := false
	for _, art := range pcResult.Artifacts {
		if art.Kind == euclotypes.ArtifactKindRecoveryTrace {
			hasRecoveryTrace = true
			break
		}
	}
	assert.True(t, hasRecoveryTrace, "should have recovery trace artifact")
}

func TestProfileControllerRecoveryExhaustedStillFails(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()

	// Primary capability fails with recovery hint pointing to a nonexistent cap.
	primary := &stubProfileCapability{
		id:       "euclo:primary",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "primary failed",
			RecoveryHint: &euclotypes.RecoveryHint{
				Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
				SuggestedCapability: "euclo:nonexistent",
			},
		},
	}
	_ = reg.Register(primary)

	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(orchestrate.AdaptCapabilityRegistry(reg), euclotypes.DefaultExecutionProfileRegistry(), euclotypes.DefaultModeRegistry(), env)
	pc := orchestrate.NewProfileController(orchestrate.AdaptCapabilityRegistry(reg), gate.DefaultPhaseGates(), env, euclotypes.DefaultExecutionProfileRegistry(), rc)

	profile := euclotypes.ExecutionProfileSelection{
		ProfileID:   "edit_verify_repair",
		PhaseRoutes: map[string]string{"explore": "react", "plan": "pipeline", "edit": "pipeline", "verify": "react"},
	}
	mode := euclotypes.ModeResolution{ModeID: "code"}

	result, pcResult, err := pc.ExecuteProfile(context.Background(), profile, mode, testEnvelope(nil))
	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, 1, pcResult.RecoveryAttempts)
}

// --- Helper extractors ---

// TestParadigmFromFailure tests the paradigmFromFailure helper function
// Note: paradigmFromFailure is a private function that needs to be implemented
// This test is commented out pending implementation
/*
func TestParadigmFromFailure(t *testing.T) {
	assert.Equal(t, "react", paradigmFromFailure(euclotypes.ExecutionResult{
		FailureInfo: &euclotypes.CapabilityFailure{ParadigmUsed: "react"},
	}))
	assert.Equal(t, "unknown", paradigmFromFailure(euclotypes.ExecutionResult{}))
}

func TestProducerIDFromFailure(t *testing.T) {
	assert.Equal(t, "cap_a", producerIDFromFailure(euclotypes.ExecutionResult{
		Artifacts: []euclotypes.Artifact{{ProducerID: "cap_a"}},
	}))
	assert.Equal(t, "err_code", producerIDFromFailure(euclotypes.ExecutionResult{
		FailureInfo: &euclotypes.CapabilityFailure{Code: "err_code"},
	}))
	assert.Equal(t, "", producerIDFromFailure(euclotypes.ExecutionResult{}))
}
*/

func TestNewRecoveryController(t *testing.T) {
	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(
		orchestrate.AdaptCapabilityRegistry(capabilities.NewEucloCapabilityRegistry()),
		euclotypes.DefaultExecutionProfileRegistry(),
		euclotypes.DefaultModeRegistry(),
		env,
	)
	require.NotNil(t, rc)
	require.NotNil(t, rc.Capabilities)
	require.NotNil(t, rc.Profiles)
	require.NotNil(t, rc.Modes)
}

func TestRecoveryParadigmSwitchFailureReturnsOriginal(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	// Capability that fails on retry too.
	stub := &stubProfileCapability{
		id:       "euclo:retry_fails",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "still failed",
		},
	}
	_ = reg.Register(stub)

	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(orchestrate.AdaptCapabilityRegistry(reg), nil, nil, env)
	stack := orchestrate.NewRecoveryStack()

	failedResult := euclotypes.ExecutionResult{
		Status:  euclotypes.ExecutionStatusFailed,
		Summary: "original failure",
		Artifacts: []euclotypes.Artifact{
			{ProducerID: "euclo:retry_fails"},
		},
		FailureInfo: &euclotypes.CapabilityFailure{ParadigmUsed: "react"},
	}

	recovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy:          euclotypes.RecoveryStrategyParadigmSwitch,
		SuggestedParadigm: "planner",
	}, failedResult, testEnvelope(nil), stack)

	// Should return original failed result when retry also fails.
	assert.Equal(t, euclotypes.ExecutionStatusFailed, recovered.Status)
	assert.Equal(t, "original failure", recovered.Summary)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
}

func TestRecoveryCapabilityFallbackFailureReturnsOriginal(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	_ = reg.Register(&stubProfileCapability{
		id:       "euclo:fallback_also_fails",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: euclotypes.ExecutionResult{
			Status:  euclotypes.ExecutionStatusFailed,
			Summary: "fallback also failed",
		},
	})

	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(orchestrate.AdaptCapabilityRegistry(reg), nil, nil, env)
	stack := orchestrate.NewRecoveryStack()

	failedResult := euclotypes.ExecutionResult{
		Status:  euclotypes.ExecutionStatusFailed,
		Summary: "original failure",
	}

	recovered := rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy:            euclotypes.RecoveryStrategyCapabilityFallback,
		SuggestedCapability: "euclo:fallback_also_fails",
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, euclotypes.ExecutionStatusFailed, recovered.Status)
	assert.Equal(t, "original failure", recovered.Summary)
}

func TestRecoveryParadigmSwitchSetsEnvelopeOverride(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	var capturedEnv euclotypes.ExecutionEnvelope
	stub := &stubProfileCapability{
		id:       "euclo:capture",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
	}
	// Override Execute to capture the envelope.
	captureCap := &capturingCapability{
		stubProfileCapability: stub,
		onExecute: func(_ context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
			capturedEnv = env
			return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted}
		},
	}
	_ = reg.Register(captureCap)

	env := testEnv(t)
	rc := orchestrate.NewRecoveryController(orchestrate.AdaptCapabilityRegistry(reg), nil, nil, env)
	stack := orchestrate.NewRecoveryStack()

	envelope := testEnvelope(nil)
	envelope.Task = &core.Task{ID: "test-task", Context: map[string]any{}}

	failedResult := euclotypes.ExecutionResult{
		Status:    euclotypes.ExecutionStatusFailed,
		Artifacts: []euclotypes.Artifact{{ProducerID: "euclo:capture"}},
	}

	rc.AttemptRecovery(context.Background(), euclotypes.RecoveryHint{
		Strategy:          euclotypes.RecoveryStrategyParadigmSwitch,
		SuggestedParadigm: "pipeline",
	}, failedResult, envelope, stack)

	assert.Equal(t, "pipeline", capturedEnv.Task.Context["euclo.paradigm_override"])
}
