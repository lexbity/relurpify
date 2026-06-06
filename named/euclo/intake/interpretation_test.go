package intake

import (
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/families"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	execution "codeburg.org/lexbit/relurpify/execution"
)

func TestBuildIntentInterpretationFromEvidence(t *testing.T) {
	evidence := &intentcontext.IntentEvidence{
		ActionType:            "review",
		Target:                "named/euclo/intake",
		Scope:                 "workspace",
		RiskLevel:             "low",
		ExpectedVerb:          "review",
		ExplicitFiles:         []string{"named/euclo/intake/normalize.go"},
		MissingFields:         []string{"route"},
		RequiresClarification: true,
		ReasonCodes:           []string{"action:review"},
	}

	interpretation := BuildIntentInterpretation(evidence, nil)
	if interpretation == nil {
		t.Fatal("expected interpretation")
	}
	if interpretation.ActionType != "review" {
		t.Fatalf("ActionType = %q, want review", interpretation.ActionType)
	}
	if interpretation.Target != "named/euclo/intake" {
		t.Fatalf("Target = %q, want named/euclo/intake", interpretation.Target)
	}
	if len(interpretation.MissingInfo) != 1 || interpretation.MissingInfo[0] != "route" {
		t.Fatalf("unexpected missing info: %#v", interpretation.MissingInfo)
	}
	if interpretation.ConfidenceNote != "deterministic interpretation from request evidence" {
		t.Fatalf("ConfidenceNote = %q, want deterministic note", interpretation.ConfidenceNote)
	}
}

func TestBuildIntentInterpretationIncludesClassificationConfidence(t *testing.T) {
	evidence := &intentcontext.IntentEvidence{
		ActionType: "implement",
		Target:     "named/euclo/intake",
		Scope:      "workspace",
		RiskLevel:  "medium",
	}
	classification := &ScoredClassification{
		WinningFamily: families.FamilyImplementation,
		Confidence:    0.81,
		Ambiguous:     false,
	}

	interpretation := BuildIntentInterpretation(evidence, classification)
	if interpretation == nil {
		t.Fatal("expected interpretation")
	}
	if interpretation.ConfidenceNote == "" {
		t.Fatal("expected confidence note")
	}
	found := false
	for _, code := range interpretation.ReasonCodes {
		if code == "classification_present" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected classification source reason code, got %#v", interpretation.ReasonCodes)
	}
}

func TestTaskEnvelopeCarriesInterpretation(t *testing.T) {
	task := &execution.Task{
		ID:          "task-1",
		Instruction: "review the intake path",
	}
	envelope, err := NormalizeTaskEnvelope(task, nil)
	if err != nil {
		t.Fatalf("NormalizeTaskEnvelope failed: %v", err)
	}
	if envelope.Interpretation == nil {
		t.Fatal("expected task envelope interpretation")
	}
	if envelope.Interpretation.ActionType == "" {
		t.Fatal("expected action type to be populated")
	}
}
