package orchestrate

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

// seedRequestText puts the route's original request on the envelope exactly the
// way the agent does (KeyTaskInput carries the *execution.Task).
func seedRequestText(t *testing.T, env *contextdata.Envelope, instruction string) {
	t.Helper()
	env.SetWorkingValueWithOrigin(state.KeyTaskInput,
		&execution.Task{ID: env.TaskIDSnapshot(), Instruction: instruction},
		contextdata.MemoryClassTask, contextdata.OriginUser)
}

// routeRequestFromEnvelope must carry the request text: the dispatcher scores
// candidates against it, so an empty Instruction silently degrades selection to
// the structured intake evidence alone.
func TestRouteRequestFromEnvelopeCarriesInstruction(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	seedRequestText(t, env, "  review the auth module  ")

	req := routeRequestFromEnvelope(env)
	if req.Instruction != "review the auth module" {
		t.Fatalf("req.Instruction = %q, want the trimmed request text", req.Instruction)
	}
}

// No task on the envelope (a synthesized default task has an empty instruction)
// leaves the field empty rather than inventing text.
func TestRouteRequestFromEnvelopeInstructionEmptyWithoutTask(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	if req := routeRequestFromEnvelope(env); req.Instruction != "" {
		t.Fatalf("req.Instruction = %q, want empty without a task", req.Instruction)
	}
}

// The selection match vocabulary drops English function words, so a recipe
// description cannot match a request on "the"/"a"/"in" alone. Content words
// survive.
func TestSelectionTokensDropFunctionWords(t *testing.T) {
	set := tokenNormalizedSet([]string{"Fix the bug in the parser"})
	for _, stop := range []string{"the", "in"} {
		if _, ok := set[stop]; ok {
			t.Errorf("function word %q must be dropped from the selection vocabulary", stop)
		}
	}
	for _, keep := range []string{"fix", "bug", "parser"} {
		if _, ok := set[keep]; !ok {
			t.Errorf("content word %q must be kept in the selection vocabulary", keep)
		}
	}
}

// A recipe whose only textual overlap with the request is function words scores
// zero: the request text is in the token set, so without the stopword filter
// this candidate would be manufactured from "the"/"in".
func TestScoreThoughtRecipeCandidateIgnoresFunctionWordOverlap(t *testing.T) {
	entry := thoughtrecipepkg.Entry{ThoughtRecipe: &surface.ThoughtRecipe{
		ID:          "custom.thoughtrecipe.stream",
		Name:        "stream",
		Description: "the stream of the a in it",
		Metadata:    surface.ThoughtRecipeMetadata{Families: []string{"unrelated"}},
	}}
	env := contextdata.NewEnvelope("task-1", "session-1")
	ctx := selectionContext{
		family: "implementation",
		tokens: routeSearchTokens(env, RouteRequest{Instruction: "explain the frobnicator in frobnicator.go"}),
	}

	if score := scoreThoughtRecipeCandidate(entry, ctx); score.total != 0 {
		t.Fatalf("function-word-only overlap scored %d (%v), want 0", score.total, score.matchedKeywords)
	}
}

// A recipe keyword that matches the request text now selects that recipe: two
// review-family siblings tie on family affinity, and the request-aware keyword
// component breaks the tie.
func TestDispatch_RequestKeywordSelectsRecipe(t *testing.T) {
	reg := thoughtrecipepkg.NewThoughtRecipeRegistry()
	if err := reg.Register(&surface.ThoughtRecipe{
		ID:       "euclo.thoughtrecipe.code_review",
		Name:     "code_review",
		Metadata: surface.ThoughtRecipeMetadata{Families: []string{families.FamilyReview}},
	}); err != nil {
		t.Fatalf("register code_review: %v", err)
	}
	if err := reg.Register(&surface.ThoughtRecipe{
		ID:       "euclo.thoughtrecipe.security_review",
		Name:     "security_review",
		Metadata: surface.ThoughtRecipeMetadata{Families: []string{families.FamilyReview}, Keywords: []string{"security", "auth"}},
	}); err != nil {
		t.Fatalf("register security_review: %v", err)
	}

	famReg := families.NewRegistry()
	if err := families.RegisterBuiltins(famReg); err != nil {
		t.Fatalf("register builtin families: %v", err)
	}

	env := contextdata.NewEnvelope("task-1", "session-1")
	state.SetFamilySelection(env, families.FamilyReview)
	seedRequestText(t, env, "review the auth module for security holes")

	req := routeRequestFromEnvelope(env)
	result, err := Dispatch(context.Background(), env, req, SelectionDeps{
		ThoughtRecipes:   reg,
		Families:         famReg,
		SelectionPosture: "deterministic",
	})
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if result.RouteID != "euclo.thoughtrecipe.security_review" {
		t.Fatalf("dispatched %q, want security_review (request keyword must outrank the sibling)", result.RouteID)
	}
}
