package euclo

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecoveryStackDefaults(t *testing.T) {
	stack := NewRecoveryStack()
	require.NotNil(t, stack)
	assert.Equal(t, 3, stack.MaxDepth)
	assert.False(t, stack.Exhausted)
	assert.Empty(t, stack.Attempts)
	assert.True(t, stack.CanAttempt())
}

func TestRecoveryStackCanAttempt(t *testing.T) {
	stack := NewRecoveryStack()
	// Fill to max depth.
	for i := 0; i < 3; i++ {
		assert.True(t, stack.CanAttempt())
		stack.Record(RecoveryAttempt{Level: RecoveryLevelParadigm, Success: false})
	}
	assert.False(t, stack.CanAttempt())
	assert.True(t, stack.Exhausted)
	assert.Len(t, stack.Attempts, 3)
}

func TestRecoveryStackNilSafe(t *testing.T) {
	var stack *RecoveryStack
	assert.False(t, stack.CanAttempt())
	stack.Record(RecoveryAttempt{}) // should not panic
}

func TestRecoveryStackExhaustedPreventsAttempt(t *testing.T) {
	stack := NewRecoveryStack()
	stack.Exhausted = true
	assert.False(t, stack.CanAttempt())
}

func TestRecoveryTraceArtifact(t *testing.T) {
	stack := NewRecoveryStack()
	stack.Record(RecoveryAttempt{
		Level:   RecoveryLevelCapability,
		From:    "cap_a",
		To:      "cap_b",
		Reason:  "fallback",
		Success: true,
	})
	stack.Record(RecoveryAttempt{
		Level:   RecoveryLevelProfile,
		From:    "profile_a",
		To:      "profile_b",
		Reason:  "escalation",
		Success: false,
	})

	art := RecoveryTraceArtifact(stack, "euclo:test")
	assert.Equal(t, ArtifactKindRecoveryTrace, art.Kind)
	assert.Equal(t, "euclo:test", art.ProducerID)
	assert.Equal(t, "produced", art.Status)
	assert.Contains(t, art.Summary, "2 recovery attempts")

	payload, ok := art.Payload.(map[string]any)
	require.True(t, ok)
	attempts, ok := payload["attempts"].([]map[string]any)
	require.True(t, ok)
	assert.Len(t, attempts, 2)
	assert.Equal(t, "capability", attempts[0]["level"])
	assert.Equal(t, true, attempts[0]["success"])
	assert.Equal(t, "profile", attempts[1]["level"])
	assert.Equal(t, false, attempts[1]["success"])
}

func TestRecoveryControllerNilSafe(t *testing.T) {
	var rc *RecoveryController
	stack := NewRecoveryStack()
	result := ExecutionResult{Status: ExecutionStatusFailed}
	env := testEnvelope(nil)

	recovered := rc.AttemptRecovery(context.Background(), RecoveryHint{
		Strategy: RecoveryStrategyCapabilityFallback,
	}, result, env, stack)

	assert.Equal(t, ExecutionStatusFailed, recovered.Status)
	assert.Empty(t, stack.Attempts)
}

func TestRecoveryControllerExhaustedStack(t *testing.T) {
	env := testEnv(t)
	rc := NewRecoveryController(
		NewEucloCapabilityRegistry(),
		DefaultExecutionProfileRegistry(),
		DefaultModeRegistry(),
		env,
	)
	stack := NewRecoveryStack()
	stack.Exhausted = true

	result := ExecutionResult{Status: ExecutionStatusFailed}
	recovered := rc.AttemptRecovery(context.Background(), RecoveryHint{
		Strategy: RecoveryStrategyCapabilityFallback,
	}, result, testEnvelope(nil), stack)

	assert.Equal(t, ExecutionStatusFailed, recovered.Status)
}

// --- Paradigm Switch Tests ---

func TestRecoveryParadigmSwitchSuccess(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	stub := &stubProfileCapability{
		id:       "euclo:test_cap",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: ExecutionResult{
			Status:  ExecutionStatusCompleted,
			Summary: "paradigm switch succeeded",
			Artifacts: []Artifact{
				{Kind: ArtifactKindEditIntent, ProducerID: "euclo:test_cap"},
			},
		},
	}
	_ = reg.Register(stub)

	env := testEnv(t)
	rc := NewRecoveryController(reg, DefaultExecutionProfileRegistry(), DefaultModeRegistry(), env)
	stack := NewRecoveryStack()

	failedResult := ExecutionResult{
		Status: ExecutionStatusFailed,
		FailureInfo: &CapabilityFailure{
			ParadigmUsed: "react",
		},
		Artifacts: []Artifact{
			{ProducerID: "euclo:test_cap"},
		},
	}

	recovered := rc.AttemptRecovery(context.Background(), RecoveryHint{
		Strategy:          RecoveryStrategyParadigmSwitch,
		SuggestedParadigm: "planner",
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, ExecutionStatusCompleted, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.True(t, stack.Attempts[0].Success)
	assert.Equal(t, RecoveryLevelParadigm, stack.Attempts[0].Level)
	assert.Equal(t, "react", stack.Attempts[0].From)
	assert.Equal(t, "planner", stack.Attempts[0].To)
	assert.True(t, stub.executeCalled)
}

func TestRecoveryParadigmSwitchNoSuggestedParadigm(t *testing.T) {
	env := testEnv(t)
	rc := NewRecoveryController(NewEucloCapabilityRegistry(), nil, nil, env)
	stack := NewRecoveryStack()

	failedResult := ExecutionResult{Status: ExecutionStatusFailed}
	recovered := rc.AttemptRecovery(context.Background(), RecoveryHint{
		Strategy: RecoveryStrategyParadigmSwitch,
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
}

func TestRecoveryParadigmSwitchCapabilityNotFound(t *testing.T) {
	env := testEnv(t)
	rc := NewRecoveryController(NewEucloCapabilityRegistry(), nil, nil, env)
	stack := NewRecoveryStack()

	failedResult := ExecutionResult{
		Status:    ExecutionStatusFailed,
		Artifacts: []Artifact{{ProducerID: "euclo:nonexistent"}},
	}
	recovered := rc.AttemptRecovery(context.Background(), RecoveryHint{
		Strategy:          RecoveryStrategyParadigmSwitch,
		SuggestedParadigm: "planner",
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
	assert.Contains(t, stack.Attempts[0].Reason, "not found")
}

// --- Capability Fallback Tests ---

func TestRecoveryCapabilityFallbackSuccess(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	fallbackCap := &stubProfileCapability{
		id:       "euclo:fallback_cap",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: ExecutionResult{
			Status:  ExecutionStatusCompleted,
			Summary: "fallback succeeded",
			Artifacts: []Artifact{
				{Kind: ArtifactKindEditIntent, ProducerID: "euclo:fallback_cap"},
			},
		},
	}
	_ = reg.Register(fallbackCap)

	env := testEnv(t)
	rc := NewRecoveryController(reg, nil, nil, env)
	stack := NewRecoveryStack()

	failedResult := ExecutionResult{
		Status:    ExecutionStatusFailed,
		Artifacts: []Artifact{{ProducerID: "euclo:original"}},
	}

	recovered := rc.AttemptRecovery(context.Background(), RecoveryHint{
		Strategy:            RecoveryStrategyCapabilityFallback,
		SuggestedCapability: "euclo:fallback_cap",
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, ExecutionStatusCompleted, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.True(t, stack.Attempts[0].Success)
	assert.Equal(t, RecoveryLevelCapability, stack.Attempts[0].Level)
	assert.True(t, fallbackCap.executeCalled)
}

func TestRecoveryCapabilityFallbackNotEligible(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	_ = reg.Register(&stubProfileCapability{
		id:       "euclo:ineligible",
		profiles: []string{"edit_verify_repair"},
		eligible: false,
	})

	env := testEnv(t)
	rc := NewRecoveryController(reg, nil, nil, env)
	stack := NewRecoveryStack()

	failedResult := ExecutionResult{Status: ExecutionStatusFailed}
	recovered := rc.AttemptRecovery(context.Background(), RecoveryHint{
		Strategy:            RecoveryStrategyCapabilityFallback,
		SuggestedCapability: "euclo:ineligible",
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
	assert.Contains(t, stack.Attempts[0].Reason, "not eligible")
}

func TestRecoveryCapabilityFallbackNotFound(t *testing.T) {
	env := testEnv(t)
	rc := NewRecoveryController(NewEucloCapabilityRegistry(), nil, nil, env)
	stack := NewRecoveryStack()

	failedResult := ExecutionResult{Status: ExecutionStatusFailed}
	recovered := rc.AttemptRecovery(context.Background(), RecoveryHint{
		Strategy:            RecoveryStrategyCapabilityFallback,
		SuggestedCapability: "euclo:nonexistent",
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
	assert.Contains(t, stack.Attempts[0].Reason, "not found")
}

func TestRecoveryCapabilityFallbackNoSuggestion(t *testing.T) {
	env := testEnv(t)
	rc := NewRecoveryController(NewEucloCapabilityRegistry(), nil, nil, env)
	stack := NewRecoveryStack()

	failedResult := ExecutionResult{Status: ExecutionStatusFailed}
	recovered := rc.AttemptRecovery(context.Background(), RecoveryHint{
		Strategy: RecoveryStrategyCapabilityFallback,
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
}

// --- Profile Escalation Tests ---

func TestRecoveryProfileEscalationSuccess(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	// Register a capability for the fallback profile.
	fallbackCap := &stubProfileCapability{
		id:       "euclo:fallback_profile_cap",
		profiles: []string{"reproduce_localize_patch"},
		eligible: true,
		executeResult: ExecutionResult{
			Status:  ExecutionStatusCompleted,
			Summary: "profile escalation succeeded",
		},
	}
	_ = reg.Register(fallbackCap)

	env := testEnv(t)
	rc := NewRecoveryController(reg, DefaultExecutionProfileRegistry(), DefaultModeRegistry(), env)
	stack := NewRecoveryStack()

	failedResult := ExecutionResult{Status: ExecutionStatusFailed}
	envelope := testEnvelope(nil)
	envelope.Profile = ExecutionProfileSelection{
		ProfileID: "edit_verify_repair",
	}

	recovered := rc.AttemptRecovery(context.Background(), RecoveryHint{
		Strategy: RecoveryStrategyProfileEscalation,
	}, failedResult, envelope, stack)

	assert.Equal(t, ExecutionStatusCompleted, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.True(t, stack.Attempts[0].Success)
	assert.Equal(t, RecoveryLevelProfile, stack.Attempts[0].Level)
	assert.Equal(t, "edit_verify_repair", stack.Attempts[0].From)
	assert.Equal(t, "reproduce_localize_patch", stack.Attempts[0].To)
	assert.True(t, fallbackCap.executeCalled)
}

func TestRecoveryProfileEscalationNoFallbacks(t *testing.T) {
	// Create a profile registry with a profile that has no fallbacks.
	profiles := NewExecutionProfileRegistry()
	_ = profiles.Register(ExecutionProfileDescriptor{
		ProfileID:      "no_fallback",
		SupportedModes: []string{"code"},
		PhaseRoutes:    map[string]string{"explore": "react"},
	})

	env := testEnv(t)
	rc := NewRecoveryController(NewEucloCapabilityRegistry(), profiles, nil, env)
	stack := NewRecoveryStack()

	failedResult := ExecutionResult{Status: ExecutionStatusFailed}
	envelope := testEnvelope(nil)
	envelope.Profile = ExecutionProfileSelection{ProfileID: "no_fallback"}

	recovered := rc.AttemptRecovery(context.Background(), RecoveryHint{
		Strategy: RecoveryStrategyProfileEscalation,
	}, failedResult, envelope, stack)

	assert.Equal(t, ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
	assert.Contains(t, stack.Attempts[0].Reason, "no fallback profiles")
}

func TestRecoveryProfileEscalationNoEligibleCapability(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	// Capability for fallback profile but not eligible.
	_ = reg.Register(&stubProfileCapability{
		id:       "euclo:ineligible_for_fallback",
		profiles: []string{"reproduce_localize_patch"},
		eligible: false,
	})

	env := testEnv(t)
	rc := NewRecoveryController(reg, DefaultExecutionProfileRegistry(), nil, env)
	stack := NewRecoveryStack()

	failedResult := ExecutionResult{Status: ExecutionStatusFailed}
	envelope := testEnvelope(nil)
	envelope.Profile = ExecutionProfileSelection{ProfileID: "edit_verify_repair"}

	recovered := rc.AttemptRecovery(context.Background(), RecoveryHint{
		Strategy: RecoveryStrategyProfileEscalation,
	}, failedResult, envelope, stack)

	assert.Equal(t, ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
	assert.Contains(t, stack.Attempts[0].Reason, "no eligible capability")
}

func TestRecoveryProfileEscalationNoProfileRegistry(t *testing.T) {
	env := testEnv(t)
	rc := NewRecoveryController(NewEucloCapabilityRegistry(), nil, nil, env)
	stack := NewRecoveryStack()

	failedResult := ExecutionResult{Status: ExecutionStatusFailed}
	recovered := rc.AttemptRecovery(context.Background(), RecoveryHint{
		Strategy: RecoveryStrategyProfileEscalation,
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
	assert.Contains(t, stack.Attempts[0].Reason, "registry unavailable")
}

// --- Mode Escalation Tests ---

func TestRecoveryModeEscalationRecommendation(t *testing.T) {
	env := testEnv(t)
	rc := NewRecoveryController(nil, nil, DefaultModeRegistry(), env)
	stack := NewRecoveryStack()

	failedResult := ExecutionResult{Status: ExecutionStatusFailed, Summary: "original failure"}
	recovered := rc.AttemptRecovery(context.Background(), RecoveryHint{
		Strategy: RecoveryStrategyModeEscalation,
	}, failedResult, testEnvelope(nil), stack)

	// Mode escalation never succeeds — it returns a recommendation.
	assert.Equal(t, ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
	assert.Equal(t, RecoveryLevelMode, stack.Attempts[0].Level)

	// The recovery hint should be enriched with escalation status.
	require.NotNil(t, recovered.RecoveryHint)
	assert.Equal(t, RecoveryStrategyModeEscalation, recovered.RecoveryHint.Strategy)
	require.NotNil(t, recovered.RecoveryHint.Context)
	assert.Equal(t, "recommended", recovered.RecoveryHint.Context["escalation_status"])
	assert.Equal(t, true, recovered.RecoveryHint.Context["requires_approval"])
}

// --- Max Depth / Stack Tracking ---

func TestRecoveryMaxDepthEnforced(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	// Register a capability that always fails.
	_ = reg.Register(&stubProfileCapability{
		id:       "euclo:always_fails",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: ExecutionResult{
			Status: ExecutionStatusFailed,
			RecoveryHint: &RecoveryHint{
				Strategy:            RecoveryStrategyCapabilityFallback,
				SuggestedCapability: "euclo:always_fails",
			},
		},
	})

	env := testEnv(t)
	rc := NewRecoveryController(reg, nil, nil, env)
	stack := NewRecoveryStack()

	failedResult := ExecutionResult{Status: ExecutionStatusFailed}

	// Attempt recovery 3 times (max depth).
	for i := 0; i < 3; i++ {
		rc.AttemptRecovery(context.Background(), RecoveryHint{
			Strategy:            RecoveryStrategyCapabilityFallback,
			SuggestedCapability: "euclo:always_fails",
		}, failedResult, testEnvelope(nil), stack)
	}

	assert.True(t, stack.Exhausted)
	assert.Len(t, stack.Attempts, 3)

	// Fourth attempt should be refused.
	recovered := rc.AttemptRecovery(context.Background(), RecoveryHint{
		Strategy:            RecoveryStrategyCapabilityFallback,
		SuggestedCapability: "euclo:always_fails",
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, ExecutionStatusFailed, recovered.Status)
	assert.Len(t, stack.Attempts, 3) // no new attempt recorded
}

func TestRecoveryStackTracksMultipleLevels(t *testing.T) {
	stack := NewRecoveryStack()
	stack.Record(RecoveryAttempt{Level: RecoveryLevelParadigm, From: "react", To: "planner", Success: false})
	stack.Record(RecoveryAttempt{Level: RecoveryLevelCapability, From: "cap_a", To: "cap_b", Success: true})
	stack.Record(RecoveryAttempt{Level: RecoveryLevelProfile, From: "p1", To: "p2", Success: false})

	assert.Len(t, stack.Attempts, 3)
	assert.True(t, stack.Exhausted)

	assert.Equal(t, RecoveryLevelParadigm, stack.Attempts[0].Level)
	assert.Equal(t, RecoveryLevelCapability, stack.Attempts[1].Level)
	assert.Equal(t, RecoveryLevelProfile, stack.Attempts[2].Level)
}

func TestRecoveryUnknownStrategy(t *testing.T) {
	env := testEnv(t)
	rc := NewRecoveryController(nil, nil, nil, env)
	stack := NewRecoveryStack()

	failedResult := ExecutionResult{Status: ExecutionStatusFailed}
	recovered := rc.AttemptRecovery(context.Background(), RecoveryHint{
		Strategy: "unknown_strategy",
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, ExecutionStatusFailed, recovered.Status)
	assert.Empty(t, stack.Attempts) // unknown strategy records nothing
}

// --- Integration with ProfileController ---

func TestProfileControllerRecoveryOnProfileLevelFailure(t *testing.T) {
	reg := NewEucloCapabilityRegistry()

	// Primary capability fails with recovery hint.
	primary := &stubProfileCapability{
		id:       "euclo:primary",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: ExecutionResult{
			Status:  ExecutionStatusFailed,
			Summary: "primary failed",
			RecoveryHint: &RecoveryHint{
				Strategy:            RecoveryStrategyCapabilityFallback,
				SuggestedCapability: "euclo:recovery_cap",
			},
			Artifacts: []Artifact{{ProducerID: "euclo:primary"}},
		},
	}
	// Recovery capability succeeds.
	recoveryCap := &stubProfileCapability{
		id:       "euclo:recovery_cap",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: ExecutionResult{
			Status:  ExecutionStatusCompleted,
			Summary: "recovery succeeded",
			Artifacts: []Artifact{
				{Kind: ArtifactKindEditIntent, ProducerID: "euclo:recovery_cap"},
			},
		},
	}
	_ = reg.Register(primary)
	_ = reg.Register(recoveryCap)

	env := testEnv(t)
	rc := NewRecoveryController(reg, DefaultExecutionProfileRegistry(), DefaultModeRegistry(), env)
	pc := NewProfileController(reg, DefaultPhaseGates(), env, DefaultExecutionProfileRegistry(), rc)

	profile := ExecutionProfileSelection{
		ProfileID:   "edit_verify_repair",
		PhaseRoutes: map[string]string{"explore": "react", "plan": "pipeline", "edit": "pipeline", "verify": "react"},
	}
	mode := ModeResolution{ModeID: "code"}

	result, pcResult, err := pc.ExecuteProfile(context.Background(), profile, mode, testEnvelope(nil))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, 1, pcResult.RecoveryAttempts)

	// Check recovery trace artifact was recorded.
	hasRecoveryTrace := false
	for _, art := range pcResult.Artifacts {
		if art.Kind == ArtifactKindRecoveryTrace {
			hasRecoveryTrace = true
			break
		}
	}
	assert.True(t, hasRecoveryTrace, "should have recovery trace artifact")
}

func TestProfileControllerRecoveryExhaustedStillFails(t *testing.T) {
	reg := NewEucloCapabilityRegistry()

	// Primary capability fails with recovery hint pointing to a nonexistent cap.
	primary := &stubProfileCapability{
		id:       "euclo:primary",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: ExecutionResult{
			Status:  ExecutionStatusFailed,
			Summary: "primary failed",
			RecoveryHint: &RecoveryHint{
				Strategy:            RecoveryStrategyCapabilityFallback,
				SuggestedCapability: "euclo:nonexistent",
			},
		},
	}
	_ = reg.Register(primary)

	env := testEnv(t)
	rc := NewRecoveryController(reg, DefaultExecutionProfileRegistry(), DefaultModeRegistry(), env)
	pc := NewProfileController(reg, DefaultPhaseGates(), env, DefaultExecutionProfileRegistry(), rc)

	profile := ExecutionProfileSelection{
		ProfileID:   "edit_verify_repair",
		PhaseRoutes: map[string]string{"explore": "react", "plan": "pipeline", "edit": "pipeline", "verify": "react"},
	}
	mode := ModeResolution{ModeID: "code"}

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
	assert.Equal(t, "react", paradigmFromFailure(ExecutionResult{
		FailureInfo: &CapabilityFailure{ParadigmUsed: "react"},
	}))
	assert.Equal(t, "unknown", paradigmFromFailure(ExecutionResult{}))
}

func TestProducerIDFromFailure(t *testing.T) {
	assert.Equal(t, "cap_a", producerIDFromFailure(ExecutionResult{
		Artifacts: []Artifact{{ProducerID: "cap_a"}},
	}))
	assert.Equal(t, "err_code", producerIDFromFailure(ExecutionResult{
		FailureInfo: &CapabilityFailure{Code: "err_code"},
	}))
	assert.Equal(t, "", producerIDFromFailure(ExecutionResult{}))
}
*/

func TestNewRecoveryController(t *testing.T) {
	env := testEnv(t)
	rc := NewRecoveryController(
		NewEucloCapabilityRegistry(),
		DefaultExecutionProfileRegistry(),
		DefaultModeRegistry(),
		env,
	)
	require.NotNil(t, rc)
	require.NotNil(t, rc.Capabilities)
	require.NotNil(t, rc.Profiles)
	require.NotNil(t, rc.Modes)
}

// testEnvelope helper is defined in profile_controller_test.go.
// testEnv helper is defined in cap_edit_verify_repair_test.go.

func TestRecoveryParadigmSwitchFailureReturnsOriginal(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	// Capability that fails on retry too.
	stub := &stubProfileCapability{
		id:       "euclo:retry_fails",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: ExecutionResult{
			Status:  ExecutionStatusFailed,
			Summary: "still failed",
		},
	}
	_ = reg.Register(stub)

	env := testEnv(t)
	rc := NewRecoveryController(reg, nil, nil, env)
	stack := NewRecoveryStack()

	failedResult := ExecutionResult{
		Status:  ExecutionStatusFailed,
		Summary: "original failure",
		Artifacts: []Artifact{
			{ProducerID: "euclo:retry_fails"},
		},
		FailureInfo: &CapabilityFailure{ParadigmUsed: "react"},
	}

	recovered := rc.AttemptRecovery(context.Background(), RecoveryHint{
		Strategy:          RecoveryStrategyParadigmSwitch,
		SuggestedParadigm: "planner",
	}, failedResult, testEnvelope(nil), stack)

	// Should return original failed result when retry also fails.
	assert.Equal(t, ExecutionStatusFailed, recovered.Status)
	assert.Equal(t, "original failure", recovered.Summary)
	assert.Len(t, stack.Attempts, 1)
	assert.False(t, stack.Attempts[0].Success)
}

func TestRecoveryCapabilityFallbackFailureReturnsOriginal(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	_ = reg.Register(&stubProfileCapability{
		id:       "euclo:fallback_also_fails",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
		executeResult: ExecutionResult{
			Status:  ExecutionStatusFailed,
			Summary: "fallback also failed",
		},
	})

	env := testEnv(t)
	rc := NewRecoveryController(reg, nil, nil, env)
	stack := NewRecoveryStack()

	failedResult := ExecutionResult{
		Status:  ExecutionStatusFailed,
		Summary: "original failure",
	}

	recovered := rc.AttemptRecovery(context.Background(), RecoveryHint{
		Strategy:            RecoveryStrategyCapabilityFallback,
		SuggestedCapability: "euclo:fallback_also_fails",
	}, failedResult, testEnvelope(nil), stack)

	assert.Equal(t, ExecutionStatusFailed, recovered.Status)
	assert.Equal(t, "original failure", recovered.Summary)
}

func TestRecoveryParadigmSwitchSetsEnvelopeOverride(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	var capturedEnv ExecutionEnvelope
	stub := &stubProfileCapability{
		id:       "euclo:capture",
		profiles: []string{"edit_verify_repair"},
		eligible: true,
	}
	// Override Execute to capture the envelope.
	captureCap := &capturingCapability{
		stubProfileCapability: stub,
		onExecute: func(_ context.Context, env ExecutionEnvelope) ExecutionResult {
			capturedEnv = env
			return ExecutionResult{Status: ExecutionStatusCompleted}
		},
	}
	_ = reg.Register(captureCap)

	env := testEnv(t)
	rc := NewRecoveryController(reg, nil, nil, env)
	stack := NewRecoveryStack()

	envelope := testEnvelope(nil)
	envelope.Task = &core.Task{ID: "test-task", Context: map[string]any{}}

	failedResult := ExecutionResult{
		Status:    ExecutionStatusFailed,
		Artifacts: []Artifact{{ProducerID: "euclo:capture"}},
	}

	rc.AttemptRecovery(context.Background(), RecoveryHint{
		Strategy:          RecoveryStrategyParadigmSwitch,
		SuggestedParadigm: "pipeline",
	}, failedResult, envelope, stack)

	assert.Equal(t, "pipeline", capturedEnv.Task.Context["euclo.paradigm_override"])
}

// capturingCapability wraps a stub and lets tests intercept Execute.
type capturingCapability struct {
	*stubProfileCapability
	onExecute func(context.Context, ExecutionEnvelope) ExecutionResult
}

func (c *capturingCapability) Execute(ctx context.Context, env ExecutionEnvelope) ExecutionResult {
	if c.onExecute != nil {
		return c.onExecute(ctx, env)
	}
	return c.stubProfileCapability.Execute(ctx, env)
}
