package promptprovider

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/prompt"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
)

func TestThoughtRecipePlanGoalProviderRendersClarificationStateView(t *testing.T) {
	env := contextdata.NewEnvelope("task-goal", "session-goal")
	clarificationState := intentcontext.NewState("task-goal", "session-goal")
	clarificationState.StateVersion = 11
	clarificationState.ActiveThoughtRecipeID = "thoughtrecipe.intent.clarify"
	clarificationState.LastCheckpointID = "checkpoint-goal"
	clarificationState.LastCheckpointSeq = 42
	evidence := &intentcontext.IntentEvidence{
		ActionType:    "analyze",
		Target:        "named/euclo/intake",
		Scope:         "workspace",
		RiskLevel:     "medium",
		ExpectedVerb:  "analyze",
		ExplicitFiles: []string{"named/euclo/intake/normalize.go"},
		ReasonCodes:   []string{"action:analyze"},
	}
	interpretation := &intentcontext.IntentInterpretation{
		ActionType:     "analyze",
		Target:         "named/euclo/intake",
		Scope:          "workspace",
		RiskLevel:      "medium",
		Rationale:      "deterministic interpretation from request evidence",
		ConfidenceNote: "deterministic interpretation from request evidence",
		ReasonCodes:    []string{"action:analyze"},
	}
	contextdata.SetTyped(env, intentcontext.IntentEvidenceKey, evidence)
	contextdata.SetTyped(env, intentcontext.IntentInterpretationKey, interpretation)
	if err := intentcontext.NewStateStore().Write(nil, env, clarificationState); err != nil {
		t.Fatalf("write clarification state: %v", err)
	}

	provider := &thoughtrecipePlanGoalProvider{}
	out := provider.Provide(prompt.NewRuntimeContext(env, "react", "thoughtrecipe").
		WithVariable("instruction", "Analyze the intake path"))

	if !strings.Contains(out.Content, "Clarification State Version: 11") {
		t.Fatalf("expected clarification state version in output, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "Active ThoughtRecipe ID: thoughtrecipe.intent.clarify") {
		t.Fatalf("expected active thoughtrecipe id in output, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "Last Checkpoint ID: checkpoint-goal") {
		t.Fatalf("expected last checkpoint id in output, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "Evidence Action Type: analyze") {
		t.Fatalf("expected evidence action type in output, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "Interpretation Confidence Note: deterministic interpretation from request evidence") {
		t.Fatalf("expected interpretation note in output, got %q", out.Content)
	}
	if strings.Contains(out.Content, "Route Kind:") || strings.Contains(out.Content, "Winning Family:") || strings.Contains(out.Content, "Capability Sequence:") {
		t.Fatalf("expected plan goal provider to avoid route-policy fields, got %q", out.Content)
	}
}
