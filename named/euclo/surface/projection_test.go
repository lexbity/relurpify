package surface

import (
	"strings"
	"testing"
)

func TestBuildRecipeProjection_Linear(t *testing.T) {
	r := &ThoughtRecipe{
		ID:   "recipe.id",
		Name: "linear-recipe",
		Metadata: ThoughtRecipeMetadata{
			Families: []string{"debug"},
		},
	}
	steps := []ThoughtRecipeStep{
		{ID: "step.1", Type: "run", Parent: ThoughtRecipeStepAgent{Paradigm: "goalcon"}, Description: "Analyze input"},
		{ID: "step.2", Type: "run", Parent: ThoughtRecipeStepAgent{Paradigm: "react"}, Description: "Process request", Dependencies: []string{"step.1"}},
		{ID: "step.3", Type: "verify", Parent: ThoughtRecipeStepAgent{Paradigm: "euclo"}, Description: "Verify result", Dependencies: []string{"step.2"}},
	}

	proj := BuildRecipeProjection(r, "", steps, nil, nil)

	if proj.RecipeID != "recipe.id" {
		t.Errorf("RecipeID = %q, want %q", proj.RecipeID, "recipe.id")
	}
	if proj.Name != "linear-recipe" {
		t.Errorf("Name = %q, want %q", proj.Name, "linear-recipe")
	}
	if proj.FamilyID != "debug" {
		t.Errorf("FamilyID = %q, want %q", proj.FamilyID, "debug")
	}
	if len(proj.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(proj.Steps))
	}
	if len(proj.Groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(proj.Groups))
	}

	// Verify step mapping.
	if proj.Steps[0].StepID != "step.1" || proj.Steps[0].Paradigm != "goalcon" {
		t.Errorf("step[0] = %+v", proj.Steps[0])
	}
	if proj.Steps[1].StepID != "step.2" || proj.Steps[1].Paradigm != "react" {
		t.Errorf("step[1] = %+v", proj.Steps[1])
	}
	if proj.Steps[2].StepID != "step.3" || proj.Steps[2].Paradigm != "euclo" {
		t.Errorf("step[2] = %+v", proj.Steps[2])
	}

	// Verify dependency edges.
	if len(proj.Steps[0].DependsOn) != 0 {
		t.Errorf("step[0] should have no deps, got %v", proj.Steps[0].DependsOn)
	}
	if len(proj.Steps[1].DependsOn) != 1 || proj.Steps[1].DependsOn[0] != "step.1" {
		t.Errorf("step[1] should depend on step.1, got %v", proj.Steps[1].DependsOn)
	}
}

func TestBuildRecipeProjection_ParallelGroup(t *testing.T) {
	r := &ThoughtRecipe{
		ID:   "parallel.id",
		Name: "parallel-recipe",
	}
	steps := []ThoughtRecipeStep{
		{ID: "intro", Type: "run", Parent: ThoughtRecipeStepAgent{Paradigm: "goalcon"}, Description: "Intro"},
		{ID: "left", Type: "run", Parent: ThoughtRecipeStepAgent{Paradigm: "react"}, Description: "Left branch"},
		{ID: "right", Type: "run", Parent: ThoughtRecipeStepAgent{Paradigm: "react"}, Description: "Right branch"},
		{ID: "merge", Type: "run", Parent: ThoughtRecipeStepAgent{Paradigm: "goalcon"}, Description: "Merge results", Dependencies: []string{"left", "right"}},
	}
	parallelGroups := []ParallelGroup{
		{ID: "fanout", Steps: []ThoughtRecipeStep{steps[1], steps[2]}, Merge: MergePolicyAll},
	}

	proj := BuildRecipeProjection(r, "", steps, parallelGroups, nil)

	if len(proj.Steps) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(proj.Steps))
	}
	if len(proj.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(proj.Groups))
	}

	g := proj.Groups[0]
	if g.GroupID != "fanout" {
		t.Errorf("group ID = %q, want %q", g.GroupID, "fanout")
	}
	if g.Kind != "parallel" {
		t.Errorf("group kind = %q, want %q", g.Kind, "parallel")
	}
	if g.Merge != "all" {
		t.Errorf("group merge = %q, want %q", g.Merge, "all")
	}
	if len(g.MemberStepIDs) != 2 {
		t.Errorf("expected 2 members, got %v", g.MemberStepIDs)
	}

	// Steps in group should have GroupID set.
	if proj.Steps[1].GroupID != "fanout" {
		t.Errorf("step[1] GroupID = %q, want %q", proj.Steps[1].GroupID, "fanout")
	}
	if proj.Steps[2].GroupID != "fanout" {
		t.Errorf("step[2] GroupID = %q, want %q", proj.Steps[2].GroupID, "fanout")
	}
	if proj.Steps[0].GroupID != "" {
		t.Errorf("step[0] (intro) should have no GroupID, got %q", proj.Steps[0].GroupID)
	}
}

func TestBuildRecipeProjection_ConditionalGroup(t *testing.T) {
	r := &ThoughtRecipe{
		ID:   "cond.id",
		Name: "conditional-recipe",
	}
	ifSteps := []ThoughtRecipeStep{
		{ID: "if.branch", Type: "run", Parent: ThoughtRecipeStepAgent{Paradigm: "goalcon"}, Description: "Primary path"},
	}
	elseSteps := []ThoughtRecipeStep{
		{ID: "else.branch", Type: "run", Parent: ThoughtRecipeStepAgent{Paradigm: "react"}, Description: "Fallback path"},
	}
	allSteps := []ThoughtRecipeStep{
		{ID: "preamble", Type: "run", Parent: ThoughtRecipeStepAgent{Paradigm: "euclo"}, Description: "Setup"},
		ifSteps[0],
		elseSteps[0],
	}
	conditionalGroups := []ConditionalGroup{
		{ID: "decision", Condition: "thoughtrecipe.branch", If: ifSteps, Else: elseSteps},
	}

	proj := BuildRecipeProjection(r, "", allSteps, nil, conditionalGroups)

	if len(proj.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(proj.Groups))
	}
	g := proj.Groups[0]
	if g.Kind != "conditional" {
		t.Errorf("group kind = %q, want %q", g.Kind, "conditional")
	}
	if g.Condition != "thoughtrecipe.branch" {
		t.Errorf("condition = %q, want %q", g.Condition, "thoughtrecipe.branch")
	}

	// Else-branch step should be marked optional.
	var ifStep, elseStep *ProjectedStep
	for i := range proj.Steps {
		if proj.Steps[i].StepID == "if.branch" {
			ifStep = &proj.Steps[i]
		}
		if proj.Steps[i].StepID == "else.branch" {
			elseStep = &proj.Steps[i]
		}
	}
	if ifStep == nil {
		t.Fatal("if.branch step not found")
	}
	if elseStep == nil {
		t.Fatal("else.branch step not found")
	}
	if ifStep.Optional {
		t.Error("if.branch should not be optional")
	}
	if !elseStep.Optional {
		t.Error("else.branch should be optional")
	}
	if ifStep.GroupID != "decision" {
		t.Errorf("if.branch GroupID = %q, want %q", ifStep.GroupID, "decision")
	}
	if elseStep.GroupID != "decision" {
		t.Errorf("else.branch GroupID = %q, want %q", elseStep.GroupID, "decision")
	}
}

func TestBuildRecipeProjection_HITLGates(t *testing.T) {
	r := &ThoughtRecipe{
		ID:   "hitl.id",
		Name: "hitl-recipe",
	}
	steps := []ThoughtRecipeStep{
		{ID: "auto", Type: "run", Parent: ThoughtRecipeStepAgent{Paradigm: "goalcon"}, Description: "Automatic"},
		{ID: "approve", Type: "run", Parent: ThoughtRecipeStepAgent{Paradigm: "react"}, Description: "Needs approval", HITL: "required"},
		{ID: "confirm", Type: "run", Parent: ThoughtRecipeStepAgent{Paradigm: "goalcon"}, Description: "Confirm change", HITL: "required"},
		{ID: "finalize", Type: "run", Parent: ThoughtRecipeStepAgent{Paradigm: "euclo"}, Description: "Finalize"},
	}

	proj := BuildRecipeProjection(r, "", steps, nil, nil)

	if len(proj.HITLGates) != 2 {
		t.Fatalf("expected 2 HITL gates, got %v", proj.HITLGates)
	}
	// HITLGates are sorted.
	if proj.HITLGates[0] != "approve" {
		t.Errorf("HITLGates[0] = %q, want %q", proj.HITLGates[0], "approve")
	}
	if proj.HITLGates[1] != "confirm" {
		t.Errorf("HITLGates[1] = %q, want %q", proj.HITLGates[1], "confirm")
	}
}

func TestBuildRecipeProjection_ToolScopes(t *testing.T) {
	r := &ThoughtRecipe{
		ID:   "tools.id",
		Name: "tool-recipe",
	}
	steps := []ThoughtRecipeStep{
		{
			ID: "scoped.step", Type: "run",
			Parent: ThoughtRecipeStepAgent{Paradigm: "react"},
			Config: map[string]any{
				"tool_scopes": []string{"file_read", "file_write"},
			},
		},
		{
			ID: "no.scope", Type: "run",
			Parent: ThoughtRecipeStepAgent{Paradigm: "goalcon"},
		},
	}

	proj := BuildRecipeProjection(r, "", steps, nil, nil)

	if len(proj.Steps[0].ToolScopes) != 2 {
		t.Errorf("expected 2 tool scopes on step[0], got %v", proj.Steps[0].ToolScopes)
	}
	if len(proj.Steps[1].ToolScopes) != 0 {
		t.Errorf("expected 0 tool scopes on step[1], got %v", proj.Steps[1].ToolScopes)
	}
}

func TestBuildRecipeProjection_SelectedRoute(t *testing.T) {
	r := &ThoughtRecipe{
		ID:        "route.id",
		Name:      "route-recipe",
		RouteKind: TriggerRouteKindIntent,
	}
	steps := []ThoughtRecipeStep{
		{ID: "step.1", Type: "run", Parent: ThoughtRecipeStepAgent{Paradigm: "goalcon"}, Description: "First"},
	}

	proj := BuildRecipeProjection(r, "intent.clarify", steps, nil, nil)

	if proj.RouteKind != "intent" {
		t.Errorf("RouteKind = %q, want %q", proj.RouteKind, "intent")
	}
	if proj.SelectedRoute != "intent.clarify" {
		t.Errorf("SelectedRoute = %q, want %q", proj.SelectedRoute, "intent.clarify")
	}
	if proj.GeneratedAt.IsZero() {
		t.Error("GeneratedAt should not be zero")
	}
}

func TestBuildRecipeProjection_EmptyRecipe(t *testing.T) {
	r := &ThoughtRecipe{ID: "empty", Name: ""}
	proj := BuildRecipeProjection(r, "", nil, nil, nil)

	if proj.RecipeID != "empty" {
		t.Errorf("RecipeID = %q, want %q", proj.RecipeID, "empty")
	}
	// EffectiveName falls back to ID when Name is empty.
	if !strings.Contains(proj.Name, "empty") {
		t.Errorf("Name should contain ID fallback, got %q", proj.Name)
	}
	if len(proj.Steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(proj.Steps))
	}
	if len(proj.Groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(proj.Groups))
	}
	if len(proj.HITLGates) != 0 {
		t.Errorf("expected 0 HITL gates, got %d", len(proj.HITLGates))
	}
}

func TestBuildRecipeProjection_GoalFallbackOrder(t *testing.T) {
	r := &ThoughtRecipe{ID: "goal.id", Name: "goal-recipe"}

	steps := []ThoughtRecipeStep{
		{ID: "desc.only", Type: "run", Parent: ThoughtRecipeStepAgent{Paradigm: "goalcon"}, Description: "From description"},
		{ID: "prompt.only", Type: "run", Parent: ThoughtRecipeStepAgent{Paradigm: "react"}, Prompt: "From prompt"},
		{ID: "both.fields", Type: "run", Parent: ThoughtRecipeStepAgent{Paradigm: "euclo"}, Description: "Description wins", Prompt: "Prompt ignored"},
	}

	proj := BuildRecipeProjection(r, "", steps, nil, nil)
	if proj.Steps[0].Goal != "From description" {
		t.Errorf("step[0].Goal = %q, want %q", proj.Steps[0].Goal, "From description")
	}
	if proj.Steps[1].Goal != "From prompt" {
		t.Errorf("step[1].Goal = %q, want %q", proj.Steps[1].Goal, "From prompt")
	}
	if proj.Steps[2].Goal != "Description wins" {
		t.Errorf("step[2].Goal = %q, want %q", proj.Steps[2].Goal, "Description wins")
	}
}
