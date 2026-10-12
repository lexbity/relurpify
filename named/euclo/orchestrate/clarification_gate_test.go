package orchestrate

import (
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

// Ambiguity with real signals clarifies — multi-match with no deterministic
// tie-breaker stops for a human (selection posture ruling 2026-10).
func TestNeedsClarificationRouteOnRealSignalAmbiguity(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	state.SetIntentClassification(env, &intake.IntentClassification{
		Ambiguous:     true,
		Confidence:    0.4,
		WinningFamily: "debug",
		Signals: []intake.ClassificationSignal{
			{Kind: intake.SignalKindKeyword, FamilyID: "debug", Source: "keyword:fix"},
			{Kind: intake.SignalKindKeyword, FamilyID: "implementation", Source: "keyword:build"},
		},
	})

	if !needsClarificationRoute(env) {
		t.Fatal("expected clarification route when classification is ambiguous on real signals")
	}
}

// Grounding does not resolve family ambiguity: a multi-match instruction
// that also names a file still clarifies.
func TestNeedsClarificationRouteWhenAmbiguousAndGrounded(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	state.SetIntentEvidence(env, &intentcontext.IntentEvidence{
		ActionType: "review",
		Target:     "calc.py",
	})
	state.SetIntentClassification(env, &intake.IntentClassification{
		Ambiguous:     true,
		Confidence:    0.4,
		WinningFamily: "debug",
		Signals: []intake.ClassificationSignal{
			{Kind: intake.SignalKindKeyword, FamilyID: "debug", Source: "keyword:fix"},
			{Kind: intake.SignalKindKeyword, FamilyID: "review", Source: "keyword:review"},
		},
	})

	if !needsClarificationRoute(env) {
		t.Fatal("expected clarification route when ambiguous even with grounded evidence")
	}
}

// Evidence incompleteness (no explicit file, action verb, or scope) is
// recipe context, never a stop condition: a confident classification routes
// even when the evidence carries missing fields.
func TestNeedsClarificationRouteIgnoresEvidenceIncompleteness(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	state.SetIntentEvidence(env, &intentcontext.IntentEvidence{
		ActionType:            "review",
		Target:                "named/euclo/orchestrate/clarification.go",
		RequiresClarification: true,
		MissingFields:         []string{"route"},
	})
	state.SetIntentClassification(env, &intake.IntentClassification{
		Ambiguous:     false,
		Confidence:    0.99,
		WinningFamily: "review",
		Signals: []intake.ClassificationSignal{
			{Kind: intake.SignalKindKeyword, FamilyID: "review", Source: "keyword:review"},
		},
	})

	if needsClarificationRoute(env) {
		t.Fatal("did not expect clarification route: evidence incompleteness is not ambiguity")
	}
}

// A classification resting only on the injected default baseline matched
// nothing: there is no ambiguity to resolve, and the lattice's default
// fallback owns the turn (D8).
func TestNeedsClarificationRouteFalseForDefaultBaselineOnly(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	state.SetIntentClassification(env, &intake.IntentClassification{
		Ambiguous:     true,
		Confidence:    0.1,
		WinningFamily: "implementation",
		Signals: []intake.ClassificationSignal{
			{Kind: intake.SignalKindDefault, FamilyID: "implementation", Source: "default_baseline"},
		},
	})

	if needsClarificationRoute(env) {
		t.Fatal("did not expect clarification route for a default-baseline-only classification")
	}
}
