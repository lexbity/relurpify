package promptprovider

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/prompt"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
)

func TestThoughtRecipeStepContextProviderRendersClarificationState(t *testing.T) {
	env := contextdata.NewEnvelope("task-clarify", "session-clarify")
	state := intentcontext.NewState("task-clarify", "session-clarify")
	state.StateVersion = 9
	state.CurrentTurnID = "turn-9"
	state.ActiveThoughtRecipeID = "thoughtrecipe.intent.clarify"
	state.LastCheckpointID = "checkpoint-9"
	state.LastCheckpointSeq = 99
	state.ConfirmedEntities = []intentcontext.ConfirmedEntity{
		{StableID: "entity-9", Name: "Envelope", Kind: intentcontext.EntityKindType},
	}
	state.ConfirmedScopes = []intentcontext.ConfirmedScope{
		{StableID: "scope-9", Name: "named/euclo/promptprovider", AnchorClass: intentcontext.AnchorClassClarifiedScope},
	}
	state.PendingProjection = []intentcontext.ProjectionIntent{
		{
			StableID:       "projection-9",
			MutationKind:   "upsert_node",
			RevisionRootID: "root-9",
			IdempotencyKey: "idem-9",
			NodeKind:       "clarified_node",
			SubjectIDs:     []string{"entity-9"},
			ObjectIDs:      []string{"entity-10"},
		},
	}
	state.AppliedMutations = []intentcontext.ProjectionRecord{
		{
			StableID:       "mutation-9",
			RevisionRootID: "root-9",
			IdempotencyKey: "idem-9",
			Result:         intentcontext.ProjectionStatusApplied,
			AppliedBy:      "test",
		},
	}
	state.GroundedAnchors = []retrieval.AnchorRef{
		{AnchorID: "anchor-9", ChunkID: "chunk-9", Term: "Envelope", Definition: "type anchor", Class: "clarified_entity", Active: true},
	}
	if err := intentcontext.NewStateStore().Write(nil, env, state); err != nil {
		t.Fatalf("write clarification state: %v", err)
	}

	provider := &thoughtrecipeStepContextProvider{}
	out := provider.Provide(prompt.NewRuntimeContext(env, "react", "thoughtrecipe").
		WithStateValue(intentcontext.ClarificationStateKey, state.Clone()).
		WithVariable("question", "Which module should be updated?"))

	if !strings.Contains(out.Content, "Clarification State Version: 9") {
		t.Fatalf("expected version in provider output, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "Current Turn ID: turn-9") {
		t.Fatalf("expected turn id in provider output, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "Active ThoughtRecipe ID: thoughtrecipe.intent.clarify") {
		t.Fatalf("expected active thoughtrecipe in provider output, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "Last Checkpoint ID: checkpoint-9") {
		t.Fatalf("expected last checkpoint in provider output, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "Confirmed Entities: Envelope [type]") {
		t.Fatalf("expected confirmed entities in provider output, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "Confirmed Scopes: named/euclo/promptprovider") {
		t.Fatalf("expected confirmed scopes in provider output, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "Pending Projection: projection-9") {
		t.Fatalf("expected pending projection in provider output, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "Applied Mutations: mutation-9") {
		t.Fatalf("expected applied mutations in provider output, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "Grounded Anchors:") {
		t.Fatalf("expected grounded anchors in provider output, got %q", out.Content)
	}
	metadata := provider.Describe()
	for _, key := range []string{
		intentcontext.ClarificationStateKey,
		intentcontext.ClarificationActiveThoughtRecipeKey,
		intentcontext.ClarificationPendingProjectionKey,
		intentcontext.ClarificationProjectedMutationsKey,
		intentcontext.ClarificationConfirmedEntitiesKey,
		intentcontext.ClarificationConfirmedScopesKey,
		intentcontext.ClarificationLastCheckpointIDKey,
		intentcontext.ClarificationLastCheckpointSeqKey,
	} {
		if !containsString(metadata.ReadsKeys, key) {
			t.Fatalf("expected metadata to include read key %q, got %#v", key, metadata.ReadsKeys)
		}
	}
}

func TestThoughtRecipeStepContextProviderReadsClarificationStateFromEnvelope(t *testing.T) {
	env := contextdata.NewEnvelope("task-clarify", "session-clarify")
	state := intentcontext.NewState("task-clarify", "session-clarify")
	state.StateVersion = 3
	state.CurrentTurnID = "turn-3"
	state.ActiveThoughtRecipeID = "thoughtrecipe.intent.resume"
	if err := intentcontext.NewStateStore().Write(nil, env, state); err != nil {
		t.Fatalf("write clarification state: %v", err)
	}

	provider := &thoughtrecipeStepContextProvider{}
	out := provider.Provide(prompt.NewRuntimeContext(env, "react", "thoughtrecipe").
		WithVariable("instruction", "Continue from the shared clarification state."))
	if !strings.Contains(out.Content, "Active ThoughtRecipe ID: thoughtrecipe.intent.resume") {
		t.Fatalf("expected provider to read clarification state from envelope, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "Current Question: Continue from the shared clarification state.") {
		t.Fatalf("expected instruction fallback in provider output, got %q", out.Content)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
