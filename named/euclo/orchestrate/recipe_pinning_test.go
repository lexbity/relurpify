package orchestrate

import (
	"context"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	euclopolicy "codeburg.org/lexbit/relurpify/named/euclo/policy"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

func pinTestRegistry(t *testing.T) *thoughtrecipepkg.ThoughtRecipeRegistry {
	t.Helper()
	reg := thoughtrecipepkg.NewThoughtRecipeRegistry()
	if err := reg.Register(&surface.ThoughtRecipe{
		ID:          "demo.thoughtrecipe.pinned",
		Name:        "pinned",
		Description: "the user's choice",
	}); err != nil {
		t.Fatalf("register pinned: %v", err)
	}
	if err := reg.Register(&surface.ThoughtRecipe{
		ID:          defaultThoughtRecipeID,
		Name:        "default",
	}); err != nil {
		t.Fatalf("register default: %v", err)
	}
	return reg
}

func pinTestDeps(reg *thoughtrecipepkg.ThoughtRecipeRegistry, posture string, m *tier2TestModel) SelectionDeps {
	return SelectionDeps{
		ThoughtRecipes:   reg,
		Tier2Model:       m,
		SelectionPosture: posture,
	}
}

// TestDispatchPinnedRecipeRunsExactlyIt pins the turn-level recipe choice: a
// registered pinned ID dispatches that recipe with DecidedBy "pinned", the
// tier-2 model is never consulted, and the lattice cannot move the choice.
func TestDispatchPinnedRecipeRunsExactlyIt(t *testing.T) {
	env := contextdata.NewEnvelope("task-pin", "session-pin")
	reg := pinTestRegistry(t)
	m := &tier2TestModel{text: `{"id":"euclo.thoughtrecipe.default","confidence":1.0}`}
	deps := pinTestDeps(reg, "model_assisted", m)

	result, err := Dispatch(context.Background(), env, RouteRequest{
		PinnedRecipeID:  "demo.thoughtrecipe.pinned",
		ThoughtRecipeID: "demo.thoughtrecipe.pinned",
		FamilyID:        "debug",
	}, deps)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result.RouteID != "demo.thoughtrecipe.pinned" {
		t.Fatalf("route = %q, want the pinned recipe", result.RouteID)
	}
	if result.DecidedBy != "pinned" {
		t.Fatalf("decidedBy = %q, want pinned", result.DecidedBy)
	}
	if m.invocationCount() != 0 {
		t.Fatal("pinned dispatch consulted the tier-2 model")
	}
}

// TestDispatchPinnedUnknownRecipeFailsLoudly pins the named-failure contract:
// a pin for an unregistered recipe fails dispatch naming the ID — it may never
// silently resolve to some other recipe.
func TestDispatchPinnedUnknownRecipeFailsLoudly(t *testing.T) {
	env := contextdata.NewEnvelope("task-pin-missing", "session-pin-missing")
	reg := pinTestRegistry(t)
	deps := pinTestDeps(reg, "deterministic", nil)

	_, err := Dispatch(context.Background(), env, RouteRequest{
		PinnedRecipeID:  "demo.thoughtrecipe.missing",
		ThoughtRecipeID: "demo.thoughtrecipe.missing",
	}, deps)
	if err == nil {
		t.Fatal("unregistered pin must fail dispatch")
	}
	if !strings.Contains(err.Error(), "demo.thoughtrecipe.missing") {
		t.Fatalf("error must name the missing pin: %v", err)
	}
}

// TestTopCandidateRefsExcludeDefaultRecipe pins the tier-2 candidate rule: the
// generic default recipe is a last-resort fallback, never a model
// disambiguation candidate (its goal drops the user's request).
func TestTopCandidateRefsExcludeDefaultRecipe(t *testing.T) {
	ranked := []CandidateRouteInfo{
		availableRecipe("demo.thoughtrecipe.review_a", 50, nil),
		{RouteID: RouteID(defaultThoughtRecipeID), RouteKind: "thoughtrecipe", Availability: RouteAvailable, RankScore: 49},
		availableRecipe("demo.thoughtrecipe.review_b", 48, nil),
	}
	refs := topCandidateRefs(ranked, 5)
	for _, ref := range refs {
		if ref.ID == defaultThoughtRecipeID {
			t.Fatal("default recipe appeared in tier-2 candidate refs")
		}
	}
	if len(refs) != 2 {
		t.Fatalf("refs = %d, want the two real candidates", len(refs))
	}
}

// TestDeterministicPostureIsTheDefault pins selectionIsDeterministic: empty
// and "deterministic" gate the model tier; only "model_assisted" opts in.
func TestDeterministicPostureIsTheDefault(t *testing.T) {
	for _, posture := range []string{"", "deterministic", "DETERMINISTIC"} {
		if !selectionIsDeterministic(posture) {
			t.Fatalf("posture %q must be deterministic", posture)
		}
	}
	if selectionIsDeterministic("model_assisted") {
		t.Fatal("model_assisted must not be deterministic")
	}
}

var _ = paradigm.Contract{}
var _ = euclopolicy.HITLBroker(nil)
