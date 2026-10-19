package orchestrate

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
)

func newGateTestRegistry(ids ...string) *thoughtrecipepkg.Registry {
	reg := thoughtrecipepkg.NewThoughtRecipeRegistry()
	for _, id := range ids {
		_, _ = reg.RegisterCompiledFirstWins(testThoughtRecipe(id), nil, "test")
	}
	return reg
}

// TestAvailableRecipeCandidateGate: registered → available candidate;
// unregistered or nil registry → no candidate at all (never an unavailable
// candidate that selection could still pick).
func TestAvailableRecipeCandidateGate(t *testing.T) {
	reg := newGateTestRegistry("euclo.thoughtrecipe.code_review")

	if candidate, ok := availableRecipeCandidate(reg, "euclo.thoughtrecipe.code_review",
		"thoughtrecipe", 10, []string{"test"}); !ok || candidate.Availability != RouteAvailable ||
		string(candidate.RouteID) != "euclo.thoughtrecipe.code_review" {
		t.Fatalf("registered recipe gate failed: ok=%v candidate=%+v", ok, candidate)
	}

	if _, ok := availableRecipeCandidate(reg, "euclo.thoughtrecipe.nonexistent", "thoughtrecipe", 10, nil); ok {
		t.Fatal("unregistered recipe must not be offered")
	}
	if _, ok := availableRecipeCandidate(nil, "euclo.thoughtrecipe.code_review", "thoughtrecipe", 10, nil); ok {
		t.Fatal("nil registry must not offer candidates")
	}
	if _, ok := availableRecipeCandidate(reg, "  ", "thoughtrecipe", 10, nil); ok {
		t.Fatal("blank id must not be offered")
	}
}

// TestUnregisteredRecipeNotSelected: with the recipe absent from the
// registry, route resolution yields no recipe candidate — the absence is
// not a mid-flight "thoughtrecipe not found" (the D2/D3 defect class).
func TestUnregisteredRecipeNotSelected(t *testing.T) {
	reg := newGateTestRegistry() // empty
	report, selected, _, ok := resolveRoute(nil, RouteRequest{Instruction: "review the diff"}, SelectionDeps{ThoughtRecipes: reg})
	if ok {
		t.Fatalf("empty registry selected %q", selected.RouteID)
	}
	if report == nil {
		t.Fatal("report missing")
	}
	for _, candidate := range report.Candidates {
		if candidate.RouteKind != "capability" && string(candidate.RouteID) == "euclo.thoughtrecipe.code_review" {
			t.Fatalf("unregistered recipe leaked into candidates: %+v", candidate)
		}
	}
}

// TestRegisteredRecipeSelected: with the canonical recipe registered, an
// explicit request selects it through the gate.
func TestRegisteredRecipeSelected(t *testing.T) {
	reg := newGateTestRegistry("euclo.thoughtrecipe.code_review")
	env := contextdata.NewEnvelope("task-gate-2", "session-gate-2")
	_, err := Dispatch(context.Background(), env, RouteRequest{ThoughtRecipeID: "euclo.thoughtrecipe.code_review"}, SelectionDeps{ThoughtRecipes: reg})
	if err != nil {
		t.Fatalf("registered recipe rejected: %v", err)
	}
}

// TestDefaultFallbackRequiresRegistration: the default execution fallback is
// offered only when the default recipe is actually registered.
func TestDefaultFallbackRequiresRegistration(t *testing.T) {
	empty := newGateTestRegistry()
	if _, ok := defaultExecutionRecipeCandidate(empty); ok {
		t.Fatal("default fallback offered without registration")
	}
	full := newGateTestRegistry("euclo.thoughtrecipe.default")
	if candidate, ok := defaultExecutionRecipeCandidate(full); !ok || candidate.Availability != RouteAvailable {
		t.Fatalf("default fallback gate failed: ok=%v candidate=%+v", ok, candidate)
	}
}

// TestClarificationRouteRequiresRegistration: the clarification candidate is
// gated on the built-in clarify recipe being registered (graph construction
// registers it; a nil registry offers nothing).
func TestClarificationRouteRequiresRegistration(t *testing.T) {
	if candidate := clarificationRouteCandidate(nil, RouteRequest{}, nil); candidate != nil {
		t.Fatalf("nil registry offered clarification candidate: %+v", candidate)
	}
	reg := newGateTestRegistry(clarificationThoughtRecipeID)
	candidate := clarificationRouteCandidate(nil, RouteRequest{}, reg)
	if candidate == nil || candidate.Availability != RouteAvailable {
		t.Fatalf("registered clarify recipe not offered: %+v", candidate)
	}
}

// TestMissingRecipeIDOnExplicitMiss: an explicit request for an unregistered
// recipe fails Dispatch with MissingRecipeID naming the recipe.
func TestMissingRecipeIDOnExplicitMiss(t *testing.T) {
	reg := newGateTestRegistry()
	env := contextdata.NewEnvelope("task-gate", "session-gate")
	_, err := Dispatch(context.Background(), env, RouteRequest{ThoughtRecipeID: "euclo.thoughtrecipe.debug_tdd_repair"}, SelectionDeps{ThoughtRecipes: reg})
	rerr, isRouteErr := err.(*RouteResolutionError)
	if !isRouteErr {
		t.Fatalf("expected RouteResolutionError, got %T (%v)", err, err)
	}
	if rerr.MissingRecipeID != "euclo.thoughtrecipe.debug_tdd_repair" {
		t.Fatalf("MissingRecipeID = %q (reason %q)", rerr.MissingRecipeID, rerr.Reason)
	}
}

// TestRouteEligibleCapability pins the D8 rank-5 posture: raw local tools are
// paradigm-invoked primitives and are NEVER direct deterministic route
// candidates; governed (relurpic/provider) capabilities are. A bare file
// mention therefore cannot preempt the built-in default-recipe fallback.
func TestRouteEligibleCapability(t *testing.T) {
	local := descriptor.CapabilityDescriptor{ID: "tool:file_list", RuntimeFamily: agentspec.CapabilityRuntimeFamilyLocalTool}
	relurpic := descriptor.CapabilityDescriptor{ID: "euclo:cap.test_run", RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic}
	provider := descriptor.CapabilityDescriptor{ID: "euclo:cap.intent.clarify", RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider}

	if routeEligibleCapability(local) {
		t.Fatal("a raw local tool must not be a direct route candidate")
	}
	if !routeEligibleCapability(relurpic) {
		t.Fatal("a relurpic capability must be a direct route candidate")
	}
	if !routeEligibleCapability(provider) {
		t.Fatal("a provider capability must be a direct route candidate")
	}
}
