package orchestrate

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
)

func TestNeedsClarificationRouteUsesEvidenceNotConfidenceThreshold(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValue("euclo.intent_evidence", &intentcontext.IntentEvidence{
		ActionType:            "review",
		Target:                "named/euclo/orchestrate/clarification.go",
		RequiresClarification: true,
		MissingFields:         []string{"route"},
	}, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.intent_classification", &struct {
		Ambiguous  bool
		Confidence float64
	}{Ambiguous: false, Confidence: 0.99}, contextdata.MemoryClassTask)

	if !needsClarificationRoute(env) {
		t.Fatal("expected clarification route when evidence requires clarification")
	}
}

func TestNeedsClarificationRouteIgnoresClassificationConfidenceWhenEvidenceIsGrounded(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValue("euclo.intent_evidence", &intentcontext.IntentEvidence{
		ActionType:    "review",
		Target:        "named/euclo/orchestrate/clarification.go",
		MissingFields: nil,
	}, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.intent_classification", &struct {
		Ambiguous  bool
		Confidence float64
	}{Ambiguous: true, Confidence: 0.1}, contextdata.MemoryClassTask)

	if needsClarificationRoute(env) {
		t.Fatal("did not expect clarification route when evidence is grounded")
	}
}
