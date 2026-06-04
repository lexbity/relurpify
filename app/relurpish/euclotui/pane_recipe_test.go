package euclotui

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

func TestRecipePaneShowsRecipeWhenSelected(t *testing.T) {
	router := NewEucloEventRouter()
	pane := NewRecipePane(router, theme.Default())

	// No recipe selected yet.
	v := pane.View()
	if !strings.Contains(v, "No recipe selected") {
		t.Errorf("expected 'No recipe selected', got: %s", v)
	}

	// Select a recipe via event.
	proj := surface.RecipeProjection{
		RecipeID:  "recipe.test",
		Name:      "Test Recipe",
		RouteKind: "capability",
		Steps: []surface.ProjectedStep{
			{StepID: "step.1", Paradigm: "goalcon", Goal: "Analyze"},
			{StepID: "step.2", Paradigm: "react", Goal: "Execute", HITL: "required", ToolScopes: []string{"file_read", "file_write"}},
			{StepID: "step.3", Paradigm: "euclo", Goal: "Verify", DependsOn: []string{"step.2"}},
		},
		HITLGates: []string{"step.2"},
		Groups: []surface.ProjectedGroup{
			{GroupID: "fanout", Kind: "parallel", MemberStepIDs: []string{"step.1"}, Merge: "all"},
		},
	}
	router.ApplyExecutionEvent(ExecutionEvent{
		Type: reporting.EventTypeRecipeSelected,
		Payload: map[string]any{
			"recipe": proj,
		},
	})

	v = pane.View()
	if v == "" || strings.Contains(v, "No recipe selected") {
		t.Fatal("expected recipe view after selection")
	}

	// Check header
	if !strings.Contains(v, "Test Recipe") {
		t.Errorf("expected recipe name, got: %s", v)
	}

	// Check route kind metadata
	if !strings.Contains(v, "capability") {
		t.Errorf("expected route kind, got: %s", v)
	}

	// Check HITL gates section
	if !strings.Contains(v, "HITL Gates") {
		t.Errorf("expected HITL Gates section, got: %s", v)
	}

	// Check groups section
	if !strings.Contains(v, "parallel") {
		t.Errorf("expected group info, got: %s", v)
	}

	// Check steps
	if !strings.Contains(v, "Analyze") {
		t.Errorf("expected step.1 goal, got: %s", v)
	}
	if !strings.Contains(v, "Execute") {
		t.Errorf("expected step.2 goal, got: %s", v)
	}
	if !strings.Contains(v, "Verify") {
		t.Errorf("expected step.3 goal, got: %s", v)
	}

	// Check tool scopes annotation
	if !strings.Contains(v, "file_read") || !strings.Contains(v, "file_write") {
		t.Errorf("expected tool scopes, got: %s", v)
	}

	// Check dependency annotation
	if !strings.Contains(v, "step.2") {
		t.Errorf("expected dependency reference, got: %s", v)
	}
}

func TestRecipePaneEmptyRecipe(t *testing.T) {
	router := NewEucloEventRouter()
	pane := NewRecipePane(router, theme.Default())

	proj := surface.RecipeProjection{
		RecipeID: "empty.recipe",
		Name:     "Empty",
	}
	router.ApplyExecutionEvent(ExecutionEvent{
		Type: reporting.EventTypeRecipeSelected,
		Payload: map[string]any{
			"recipe": proj,
		},
	})

	v := pane.View()
	if !strings.Contains(v, "Empty") {
		t.Errorf("expected recipe name in view, got: %s", v)
	}
}

func TestRecipePaneWithConditionalGroup(t *testing.T) {
	router := NewEucloEventRouter()
	pane := NewRecipePane(router, theme.Default())

	proj := surface.RecipeProjection{
		RecipeID: "recipe.conditional",
		Name:     "Conditional",
		Steps: []surface.ProjectedStep{
			{StepID: "if.branch", Paradigm: "goalcon", Goal: "Primary", Optional: false},
			{StepID: "else.branch", Paradigm: "react", Goal: "Fallback", Optional: true},
		},
		Groups: []surface.ProjectedGroup{
			{GroupID: "decision", Kind: "conditional", MemberStepIDs: []string{"if.branch", "else.branch"}, Condition: "thoughtrecipe.branch"},
		},
	}
	router.ApplyExecutionEvent(ExecutionEvent{
		Type: reporting.EventTypeRecipeSelected,
		Payload: map[string]any{"recipe": proj},
	})

	v := pane.View()
	if !strings.Contains(v, "conditional") {
		t.Errorf("expected conditional group, got: %s", v)
	}
	if !strings.Contains(v, "optional") {
		t.Errorf("expected optional annotation for else branch, got: %s", v)
	}
}

func TestRecipePaneNilSafe(t *testing.T) {
	var pane *RecipePane
	v := pane.View()
	if v != "" {
		t.Errorf("nil pane should return empty, got: %q", v)
	}

	pane = NewRecipePane(nil, theme.Default())
	v = pane.View()
	if v != "" {
		t.Errorf("pane with nil router should return empty, got: %q", v)
	}
}

func TestRecipePaneUpdateOps(t *testing.T) {
	pane := NewRecipePane(NewEucloEventRouter(), theme.Default())
	pane.SetSize(100, 50)
	pane.SetStore(nil)
	pane.SetActiveTab("")
	pane.SetFilter("")
	pane.Refresh()
	pane.Cleanup()
	pane.FocusFilescopes()
	pane.OpenSecurityGuard()
	pane.OpenAIProvider()
	pane.OpenKeybindings()
	pane.OpenDoctor()

	next, cmd := pane.Update(nil)
	if next != pane {
		t.Error("Update should return self")
	}
	if cmd != nil {
		t.Error("Update should return nil cmd")
	}
	c := pane.HandleInputSubmit("")
	if c != nil {
		t.Error("HandleInputSubmit should return nil")
	}
}
