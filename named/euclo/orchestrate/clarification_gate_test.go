package orchestrate

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

func TestNeedsClarificationRouteUsesEvidenceNotConfidenceThreshold(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	state.SetIntentEvidence(env, &intentcontext.IntentEvidence{
		ActionType:            "review",
		Target:                "named/euclo/orchestrate/clarification.go",
		RequiresClarification: true,
		MissingFields:         []string{"route"},
	})
	state.SetIntentClassification(env, &intake.IntentClassification{
		Ambiguous:  false,
		Confidence: 0.99,
	})

	if !needsClarificationRoute(env) {
		t.Fatal("expected clarification route when evidence requires clarification")
	}
}

func TestNeedsClarificationRouteIgnoresClassificationConfidenceWhenEvidenceIsGrounded(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	state.SetIntentEvidence(env, &intentcontext.IntentEvidence{
		ActionType:    "review",
		Target:        "named/euclo/orchestrate/clarification.go",
		MissingFields: nil,
	})
	state.SetIntentClassification(env, &intake.IntentClassification{
		Ambiguous:  true,
		Confidence: 0.1,
	})

	if needsClarificationRoute(env) {
		t.Fatal("did not expect clarification route when evidence is grounded")
	}
}
