package orchestrate

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
)

func TestClarificationCapability_RequestWritesClarificationRequest(t *testing.T) {
	env := contextdata.NewEnvelope("task-clarify", "session-clarify")
	task := &core.Task{
		ID:          "task-clarify",
		Type:        "euclo",
		Instruction: "clarify the target module",
		Context:     map[string]any{},
		Metadata:    map[string]any{},
	}
	env.SetWorkingValue("task.input", task, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.intent_classification", &intake.IntentClassification{
		WinningFamily: "implementation",
		FamilyCandidates: []families.FamilyCandidate{
			{FamilyID: "implementation"},
			{FamilyID: "review"},
		},
	}, contextdata.MemoryClassTask)
	state := intentcontext.NewState("task-clarify", "session-clarify")
	state.Ambiguity = &intentcontext.AmbiguityCharacterization{
		Kind:       intentcontext.AmbiguityKindMultiMatch,
		Confidence: 0.25,
		Rationale:  "multiple plausible targets",
	}
	if err := intentcontext.NewStateStore().Write(context.Background(), env, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	handler := &clarificationCapabilityHandler{}
	result, err := handler.Invoke(context.Background(), env, map[string]any{
		clarificationActionKey: clarificationActionRequest,
		"max_tokens":           128,
	})
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful clarification request, got %+v", result)
	}
	if _, ok := env.GetWorkingValue(clarificationRequestKey); !ok {
		t.Fatal("expected clarification request in envelope")
	}
	if got := mustEnvelopeString(t, env, intentcontext.ClarificationActiveRecipeKey); got != clarificationRecipeID {
		t.Fatalf("active recipe id = %q, want %q", got, clarificationRecipeID)
	}
	updated, err := intentcontext.NewStateStore().Read(context.Background(), env)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if updated == nil || updated.Ambiguity == nil {
		t.Fatal("expected clarification ambiguity to be persisted")
	}
	if len(updated.Ambiguity.CandidateFamilies) < 2 {
		t.Fatalf("expected seeded candidate families, got %#v", updated.Ambiguity.CandidateFamilies)
	}
	if updated.Ambiguity.CandidateFamilies[0] != "implementation" {
		t.Fatalf("first candidate family = %q, want implementation", updated.Ambiguity.CandidateFamilies[0])
	}
}

func TestClarificationCapability_HandoffSelectsNormalRecipe(t *testing.T) {
	env := contextdata.NewEnvelope("task-handoff", "session-handoff")
	state := intentcontext.NewState("task-handoff", "session-handoff")
	state.Ambiguity = &intentcontext.AmbiguityCharacterization{
		Kind:              intentcontext.AmbiguityKindMultiMatch,
		Confidence:        0.2,
		Rationale:         "multiple plausible review targets",
		CandidateFamilies: []string{"review"},
	}
	if err := intentcontext.NewStateStore().Write(context.Background(), env, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	handler := &clarificationCapabilityHandler{}
	result, err := handler.Invoke(context.Background(), env, map[string]any{
		clarificationActionKey: clarificationActionHandoff,
		"family_id":            "review",
	})
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("expected successful handoff, got %+v", result)
	}
	if got := mustEnvelopeString(t, env, "euclo.clarification.next_recipe_id"); got != "euclo.recipe.code_review" {
		t.Fatalf("next recipe id = %q, want euclo.recipe.code_review", got)
	}
	if got := mustEnvelopeString(t, env, intentcontext.ClarificationActiveRecipeKey); got != "euclo.recipe.code_review" {
		t.Fatalf("active recipe id = %q, want euclo.recipe.code_review", got)
	}
	selection, ok := env.GetWorkingValue("euclo.route_selection")
	if !ok {
		t.Fatal("expected route_selection in envelope")
	}
	routeSelection, ok := selection.(*RouteSelection)
	if !ok || routeSelection == nil {
		t.Fatalf("expected *RouteSelection, got %T", selection)
	}
	if routeSelection.RecipeID != "euclo.recipe.code_review" {
		t.Fatalf("route selection recipe id = %q, want euclo.recipe.code_review", routeSelection.RecipeID)
	}
}

func TestValidateStructuredGroundingRejectsMissingRequiredFields(t *testing.T) {
	issues := validateStructuredGrounding(map[string]any{})
	if len(issues) == 0 {
		t.Fatal("expected grounding validation to reject missing keys")
	}
}

func mustEnvelopeString(t *testing.T, env *contextdata.Envelope, key string) string {
	t.Helper()
	value, ok := env.GetWorkingValue(key)
	if !ok {
		t.Fatalf("missing envelope value %q", key)
	}
	s, ok := value.(string)
	if !ok {
		t.Fatalf("envelope value %q is %T, want string", key, value)
	}
	return s
}
