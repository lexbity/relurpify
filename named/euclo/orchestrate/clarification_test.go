package orchestrate

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
	execution "codeburg.org/lexbit/relurpify/execution"
)

func TestClarificationCapability_RequestWritesClarificationRequest(t *testing.T) {
	env := contextdata.NewEnvelope("task-clarify", "session-clarify")
	task := &execution.Task{
		ID:          "task-clarify",
		Type:        "euclo",
		Instruction: "clarify the target module",
		Context:     map[string]any{},
		Metadata:    map[string]any{},
	}
	contextdata.SetTyped(env, "task.input", task)
	state.SetIntentClassification(env, &intake.IntentClassification{
		WinningFamily: "implementation",
		FamilyCandidates: []families.FamilyCandidate{
			{FamilyID: "implementation"},
			{FamilyID: "review"},
		},
	})
	clarificationState := intentcontext.NewState("task-clarify", "session-clarify")
	clarificationState.Ambiguity = &intentcontext.AmbiguityCharacterization{
		Kind:       intentcontext.AmbiguityKindMultiMatch,
		Confidence: 0.25,
		Rationale:  "multiple plausible targets",
	}
	if err := intentcontext.NewStateStore().Write(context.Background(), env, clarificationState); err != nil {
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
	if _, ok := contextdata.GetTyped[any](env, clarificationRequestKey); !ok {
		t.Fatal("expected clarification request in envelope")
	}
	if got, ok := contextdata.GetTyped[*ClarificationFrame](env, "euclo.interaction.clarification_frame"); !ok {
		t.Fatal("expected clarification frame in envelope")
	} else if frame := got; frame == nil || !frame.Pending() {
		t.Fatalf("unexpected clarification frame: %#v", got)
	}
	if got, ok := contextdata.GetTyped[map[string]any](env, state.KeyClarificationGateResult); !ok {
		t.Fatal("expected clarification gate result in envelope")
	} else if result := got; result["decision"] != "clarify" {
		t.Fatalf("unexpected gate result: %#v", got)
	}
	if got := mustEnvelopeString(t, env, intentcontext.ClarificationActiveThoughtRecipeKey); got != clarificationThoughtRecipeID {
		t.Fatalf("active thoughtrecipe id = %q, want %q", got, clarificationThoughtRecipeID)
	}
	routeSelection, ok := state.GetRouteSelection(env)
	if !ok || routeSelection == nil {
		t.Fatalf("expected *euclotypes.RouteSelection, got %#v", routeSelection)
	}
	if routeSelection.RouteKind != euclotypes.RouteKindIntent || routeSelection.ThoughtRecipeID != clarificationThoughtRecipeID {
		t.Fatalf("unexpected route selection: %+v", routeSelection)
	}
	if frameValue, ok := contextdata.GetTyped[*ClarificationFrame](env, "euclo.interaction.clarification_frame"); !ok || frameValue == nil {
		t.Fatalf("expected *ClarificationFrame, got %#v", frameValue)
	} else if interactionFrame := frameValue.ToInteractionFrame(); interactionFrame == nil || interactionFrame.Type != interaction.FrameIntentClarification {
		t.Fatalf("unexpected interaction frame: %#v", interactionFrame)
	}
	if meta, ok := state.GetRouteContinuation(env); !ok || meta == nil || !meta.SharedContext || meta.TargetRouteID != clarificationThoughtRecipeID {
		t.Fatalf("unexpected route continuation metadata: %#v", meta)
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

func TestClarificationCapability_HandoffSelectsNormalThoughtRecipe(t *testing.T) {
	env := contextdata.NewEnvelope("task-handoff", "session-handoff")
	clarificationState := intentcontext.NewState("task-handoff", "session-handoff")
	clarificationState.Ambiguity = &intentcontext.AmbiguityCharacterization{
		Kind:              intentcontext.AmbiguityKindMultiMatch,
		Confidence:        0.2,
		Rationale:         "multiple plausible review targets",
		CandidateFamilies: []string{"review"},
	}
	if err := intentcontext.NewStateStore().Write(context.Background(), env, clarificationState); err != nil {
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
	if got := mustEnvelopeString(t, env, "euclo.clarification.next_thoughtrecipe_id"); got != "euclo.thoughtrecipe.code_review" {
		t.Fatalf("next thoughtrecipe id = %q, want euclo.thoughtrecipe.code_review", got)
	}
	if got := mustEnvelopeString(t, env, intentcontext.ClarificationActiveThoughtRecipeKey); got != "euclo.thoughtrecipe.code_review" {
		t.Fatalf("active thoughtrecipe id = %q, want euclo.thoughtrecipe.code_review", got)
	}
	updated, err := intentcontext.NewStateStore().Read(context.Background(), env)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if updated == nil || updated.ActiveThoughtRecipeID != "euclo.thoughtrecipe.code_review" {
		t.Fatalf("expected persisted active thoughtrecipe id, got %#v", updated)
	}
	routeSelection, ok := state.GetRouteSelection(env)
	if !ok || routeSelection == nil {
		t.Fatalf("expected *euclotypes.RouteSelection, got %#v", routeSelection)
	}
	if routeSelection.RouteKind != euclotypes.RouteKindThoughtRecipe || routeSelection.ThoughtRecipeID != "euclo.thoughtrecipe.code_review" {
		t.Fatalf("route selection = %+v, want thoughtrecipe handoff", routeSelection)
	}
	meta, ok := state.GetRouteContinuation(env)
	if !ok || meta == nil {
		t.Fatalf("expected *euclotypes.RouteContinuation, got %#v", meta)
	}
	if !meta.SharedContext || meta.TargetRouteID != "euclo.thoughtrecipe.code_review" || meta.TargetRouteKind != euclotypes.RouteKindThoughtRecipe {
		t.Fatalf("unexpected continuation metadata: %+v", meta)
	}
}

func TestClarificationCapability_HandoffWithoutTargetRemainsUnresolved(t *testing.T) {
	env := contextdata.NewEnvelope("task-unresolved", "session-unresolved")
	state := intentcontext.NewState("task-unresolved", "session-unresolved")
	if err := intentcontext.NewStateStore().Write(context.Background(), env, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	handler := &clarificationCapabilityHandler{}
	result, err := handler.Invoke(context.Background(), env, map[string]any{
		clarificationActionKey: clarificationActionHandoff,
	})
	if err == nil {
		t.Fatal("expected unresolved clarification handoff error")
	}
	if result == nil || result.Success {
		t.Fatalf("expected unsuccessful unresolved handoff, got %+v", result)
	}
	if got := mustEnvelopeString(t, env, "euclo.clarification.unresolved_reason"); got != "missing handoff target" {
		t.Fatalf("unresolved reason = %q", got)
	}
	if got := mustEnvelopeString(t, env, intentcontext.ClarificationActiveThoughtRecipeKey); got != clarificationThoughtRecipeID {
		t.Fatalf("active thoughtrecipe id = %q, want %q", got, clarificationThoughtRecipeID)
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
