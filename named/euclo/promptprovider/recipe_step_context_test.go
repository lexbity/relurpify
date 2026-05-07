package promptprovider

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/prompt"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
)

func TestRecipeStepContextProviderRendersClarificationState(t *testing.T) {
	env := contextdata.NewEnvelope("task-clarify", "session-clarify")
	state := intentcontext.NewState("task-clarify", "session-clarify")
	state.StateVersion = 9
	state.CurrentTurnID = "turn-9"
	state.ActiveRecipeID = "recipe.intent.clarify"
	state.GroundedAnchors = []retrieval.AnchorRef{
		{AnchorID: "anchor-9", ChunkID: "chunk-9", Term: "Envelope", Definition: "type anchor", Class: "clarified_entity", Active: true},
	}
	if err := intentcontext.NewStateStore().Write(nil, env, state); err != nil {
		t.Fatalf("write clarification state: %v", err)
	}

	provider := &recipeStepContextProvider{}
	out := provider.Provide(prompt.NewRuntimeContext(env, "react", "recipe").
		WithStateValue(intentcontext.ClarificationStateKey, state.Clone()).
		WithVariable("question", "Which module should be updated?"))

	if !strings.Contains(out.Content, "Clarification State Version: 9") {
		t.Fatalf("expected version in provider output, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "Current Turn ID: turn-9") {
		t.Fatalf("expected turn id in provider output, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "Grounded Anchors:") {
		t.Fatalf("expected grounded anchors in provider output, got %q", out.Content)
	}
}
