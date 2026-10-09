package state

import (
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
)

func TestStepFailureRoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-fail", "session-fail")

	if _, ok := GetStepFailure(env); ok {
		t.Fatal("expected no step failure on a fresh envelope")
	}
	if failures := GetStepFailures(env); len(failures) != 0 {
		t.Fatalf("expected no failures on a fresh envelope, got %d", len(failures))
	}

	first := &euclotypes.StepFailure{Kind: euclotypes.FailureGroundingFailed, Message: "barrier failed", Cause: errors.New("wrapped")}
	SetStepFailure(env, first)
	second := &euclotypes.StepFailure{Kind: euclotypes.FailureModelUnavailable, Message: "provider down"}
	SetStepFailure(env, second)

	latest, ok := GetStepFailure(env)
	if !ok || latest == nil || latest.Kind != euclotypes.FailureModelUnavailable {
		t.Fatalf("latest step failure = %+v (ok=%v), want model_unavailable", latest, ok)
	}
	all := GetStepFailures(env)
	if len(all) != 2 {
		t.Fatalf("step failures = %d, want 2", len(all))
	}
	if all[0].Kind != euclotypes.FailureGroundingFailed || all[1].Kind != euclotypes.FailureModelUnavailable {
		t.Fatalf("step failure ordering = %s,%s, want grounding_failed,model_unavailable", all[0].Kind, all[1].Kind)
	}

	SetStepFailure(env, nil)
	if latest, ok := GetStepFailure(env); ok && latest == nil {
		t.Fatal("nil failure must not be recorded")
	}
}

func TestFallbackTakenRoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-fb", "session-fb")
	if GetFallbackTaken(env) {
		t.Fatal("expected fallback not taken on a fresh envelope")
	}
	SetFallbackTaken(env, true)
	if !GetFallbackTaken(env) {
		t.Fatal("expected fallback_taken to read back true")
	}
}