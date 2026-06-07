package euclotui

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

// recipeHelpers provides common recipe projections for integration tests.
var recipeHelpers = struct {
	linear      *surface.RecipeProjection
	parallel    *surface.RecipeProjection
	conditional *surface.RecipeProjection
	hitl        *surface.RecipeProjection
}{
	linear: &surface.RecipeProjection{
		RecipeID: "recipe.linear", Name: "Linear Recipe", RouteKind: "capability",
		Steps: []surface.ProjectedStep{
			{StepID: "step.1", Paradigm: "react", Goal: "Analyze"},
			{StepID: "step.2", Paradigm: "goalcon", Goal: "Plan"},
			{StepID: "step.3", Paradigm: "euclo", Goal: "Verify"},
		},
	},
	parallel: &surface.RecipeProjection{
		RecipeID: "recipe.parallel", Name: "Parallel Recipe", RouteKind: "capability",
		Steps: []surface.ProjectedStep{
			{StepID: "preamble", Paradigm: "euclo", Goal: "Setup"},
			{StepID: "left", Paradigm: "react", Goal: "Left branch"},
			{StepID: "right", Paradigm: "react", Goal: "Right branch"},
			{StepID: "merge", Paradigm: "goalcon", Goal: "Merge"},
		},
		Groups: []surface.ProjectedGroup{
			{GroupID: "fanout", Kind: "parallel", MemberStepIDs: []string{"left", "right"}, Merge: "all"},
		},
	},
	conditional: &surface.RecipeProjection{
		RecipeID: "recipe.conditional", Name: "Conditional Recipe", RouteKind: "intent",
		Steps: []surface.ProjectedStep{
			{StepID: "gate", Paradigm: "euclo", Goal: "Evaluate condition"},
			{StepID: "if.path", Paradigm: "goalcon", Goal: "Primary path"},
			{StepID: "else.path", Paradigm: "react", Goal: "Fallback path", Optional: true},
		},
		Groups: []surface.ProjectedGroup{
			{GroupID: "decision", Kind: "conditional", MemberStepIDs: []string{"if.path", "else.path"}, Condition: "feature_flag"},
		},
	},
	hitl: &surface.RecipeProjection{
		RecipeID: "recipe.hitl", Name: "HITL Recipe", RouteKind: "capability",
		Steps: []surface.ProjectedStep{
			{StepID: "auto", Paradigm: "goalcon", Goal: "Automatic step"},
			{StepID: "approve", Paradigm: "react", Goal: "Needs approval", HITL: "required"},
			{StepID: "finalize", Paradigm: "euclo", Goal: "Finalize after approval"},
		},
		HITLGates: []string{"approve"},
	},
}

func feedRecipe(t *testing.T, router *EucloEventRouter, proj *surface.RecipeProjection) {
	t.Helper()
	router.ApplyExecutionEvent(ExecutionEvent{
		Type:    reporting.EventTypeRecipeSelected,
		Payload: map[string]any{"recipe": *proj},
	})
}

func feedStepStarted(t *testing.T, router *EucloEventRouter, stepID, paradigm string, index, total int) {
	t.Helper()
	router.ApplyExecutionEvent(ExecutionEvent{
		Type:     reporting.EventTypeStepStartedEuclo,
		StepID:   stepID,
		Paradigm: paradigm,
		Index:    index,
		Total:    total,
	})
}

func feedStepCompleted(t *testing.T, router *EucloEventRouter, stepID, paradigm string, index, total int, success bool, durMs int64) {
	t.Helper()
	router.ApplyExecutionEvent(ExecutionEvent{
		Type:       reporting.EventTypeStepCompletedEuclo,
		StepID:     stepID,
		Paradigm:   paradigm,
		Index:      index,
		Total:      total,
		Success:    success,
		DurationMs: durMs,
	})
}

func checkSnapshot(t *testing.T, router *EucloEventRouter, wantMacro surface.MacroPhase, wantStepCount int) {
	t.Helper()
	snap := router.Snapshot()
	if snap.Macro != wantMacro {
		t.Errorf("macro = %v, want %v", snap.Macro, wantMacro)
	}
	if len(snap.StepRuntime) != wantStepCount {
		t.Errorf("step runtime count = %d, want %d", len(snap.StepRuntime), wantStepCount)
	}
}

// ── Linear recipe ──────────────────────────────────────────────────────────

func TestIntegrationLinearRecipe(t *testing.T) {
	router := NewEucloEventRouter()
	feedRecipe(t, router, recipeHelpers.linear)

	feedStepStarted(t, router, "step.1", "react", 0, 3)
	checkSnapshot(t, router, surface.MacroExecute, 1)

	feedStepCompleted(t, router, "step.1", "react", 0, 3, true, 100)
	checkSnapshot(t, router, surface.MacroExecute, 1)

	rt := router.Snapshot().StepRuntime["step.1"]
	if rt.Status != surface.StepDone || rt.DurationMs != 100 {
		t.Errorf("step.1 status/dur = %v/%d, want done/100", rt.Status, rt.DurationMs)
	}

	feedStepStarted(t, router, "step.2", "goalcon", 1, 3)
	feedStepCompleted(t, router, "step.2", "goalcon", 1, 3, true, 50)
	feedStepStarted(t, router, "step.3", "euclo", 2, 3)
	feedStepCompleted(t, router, "step.3", "euclo", 2, 3, true, 25)

	checkSnapshot(t, router, surface.MacroExecute, 3)

	// Verify rendered output contains all steps.
	stepper := NewStepper(router.Snapshot().Recipe, router.Snapshot().StepRuntime, router.Snapshot().Macro)
	rendered := stepper.Render(theme.Default())
	if !strings.Contains(rendered, "Analyze") || !strings.Contains(rendered, "Plan") || !strings.Contains(rendered, "Verify") {
		t.Errorf("stepper render missing steps:\n%s", rendered)
	}

	// Verify recipe pane shows all steps.
	pane := NewRecipePane(router, theme.Default())
	paneView := pane.View()
	if !strings.Contains(paneView, "step.1") || !strings.Contains(paneView, "step.2") || !strings.Contains(paneView, "step.3") {
		t.Errorf("recipe pane missing steps:\n%s", paneView)
	}

	// Verify resume roundtrip preserves state.
	data := router.ResumeData()
	newRouter := NewEucloEventRouter()
	newRouter.ApplyResumeData(data, nil)
	snap := newRouter.Snapshot()
	if len(snap.StepRuntime) != 3 {
		t.Errorf("resume step runtime count = %d, want 3", len(snap.StepRuntime))
	}
	if snap.StepRuntime["step.1"].Status != surface.StepDone {
		t.Errorf("resume step.1 status = %v", snap.StepRuntime["step.1"].Status)
	}
}

// ── Parallel recipe ────────────────────────────────────────────────────────

func TestIntegrationParallelRecipe(t *testing.T) {
	router := NewEucloEventRouter()
	feedRecipe(t, router, recipeHelpers.parallel)

	feedStepStarted(t, router, "preamble", "euclo", 0, 4)
	feedStepCompleted(t, router, "preamble", "euclo", 0, 4, true, 10)
	feedStepStarted(t, router, "left", "react", 1, 4)
	feedStepStarted(t, router, "right", "react", 2, 4)

	snap := router.Snapshot()
	if snap.StepRuntime["left"].Status != surface.StepActive {
		t.Errorf("left should be active")
	}
	if snap.StepRuntime["right"].Status != surface.StepActive {
		t.Errorf("right should be active")
	}
	if snap.Recipe == nil || len(snap.Recipe.Groups) == 0 {
		t.Errorf("expected group topology in snapshot")
	}

	feedStepCompleted(t, router, "left", "react", 1, 4, true, 200)
	feedStepCompleted(t, router, "right", "react", 2, 4, true, 150)
	feedStepStarted(t, router, "merge", "goalcon", 3, 4)
	feedStepCompleted(t, router, "merge", "goalcon", 3, 4, true, 30)

	checkSnapshot(t, router, surface.MacroExecute, 4)

	// Stepper render shows parallel steps.
	stepper := NewStepper(router.Snapshot().Recipe, router.Snapshot().StepRuntime, router.Snapshot().Macro)
	rendered := stepper.Render(theme.Default())
	if !strings.Contains(rendered, "Left branch") && !strings.Contains(rendered, "Right branch") {
		t.Errorf("stepper missing parallel steps:\n%s", rendered)
	}

	// Recipe pane shows group topology.
	pane := NewRecipePane(router, theme.Default())
	paneView := pane.View()
	if !strings.Contains(paneView, "parallel") || !strings.Contains(paneView, "fanout") {
		t.Errorf("recipe pane missing parallel group:\n%s", paneView)
	}
}

// ── Conditional recipe with skipped branch ─────────────────────────────────

func TestIntegrationConditionalRecipe(t *testing.T) {
	router := NewEucloEventRouter()
	feedRecipe(t, router, recipeHelpers.conditional)

	feedStepStarted(t, router, "gate", "euclo", 0, 3)
	feedStepCompleted(t, router, "gate", "euclo", 0, 3, true, 5)
	feedStepStarted(t, router, "if.path", "goalcon", 1, 3)
	feedStepCompleted(t, router, "if.path", "goalcon", 1, 3, true, 80)

	// Simulate branch resolution that skips the else path.
	router.ApplyExecutionEvent(ExecutionEvent{
		Type: reporting.EventTypeBranchResolved,
		Payload: map[string]any{
			"skipped_step_ids": []string{"else.path"},
		},
	})

	snap := router.Snapshot()
	rt, ok := snap.StepRuntime["else.path"]
	if !ok {
		t.Fatal("else.path should exist in runtime")
	}
	if rt.Status != surface.StepSkipped {
		t.Errorf("else.path status = %v, want %v", rt.Status, surface.StepSkipped)
	}

	// Stepper shows skipped step.
	stepper := NewStepper(snap.Recipe, snap.StepRuntime, snap.Macro)
	rendered := stepper.Render(theme.Default())
	if !strings.Contains(rendered, "Fallback path") {
		t.Errorf("stepper should show skipped fallback:\n%s", rendered)
	}
}

// ── HITL-gated recipe ──────────────────────────────────────────────────────

func TestIntegrationHITLRecipe(t *testing.T) {
	router := NewEucloEventRouter()
	feedRecipe(t, router, recipeHelpers.hitl)

	feedStepStarted(t, router, "auto", "goalcon", 0, 3)
	feedStepCompleted(t, router, "auto", "goalcon", 0, 3, true, 30)

	snap := router.Snapshot()
	if snap.Recipe == nil || len(snap.Recipe.HITLGates) != 1 || snap.Recipe.HITLGates[0] != "approve" {
		t.Errorf("HITL gates = %v, want [approve]", snap.Recipe.HITLGates)
	}

	feedStepStarted(t, router, "approve", "react", 1, 3)
	feedStepCompleted(t, router, "approve", "react", 1, 3, true, 5000)
	feedStepStarted(t, router, "finalize", "euclo", 2, 3)
	feedStepCompleted(t, router, "finalize", "euclo", 2, 3, true, 10)

	checkSnapshot(t, router, surface.MacroExecute, 3)

	// Recipe pane shows HITL gates section.
	pane := NewRecipePane(router, theme.Default())
	paneView := pane.View()
	if !strings.Contains(paneView, "HITL Gates") || !strings.Contains(paneView, "approve") {
		t.Errorf("recipe pane missing HITL gates:\n%s", paneView)
	}

	// Verify the step with HITL annotation.
	if !strings.Contains(paneView, "HITL: required") {
		t.Errorf("expected HITL annotation on approve step:\n%s", paneView)
	}
}

// ── Multi-paradigm recipe ──────────────────────────────────────────────────

func TestIntegrationMultiParadigmRecipe(t *testing.T) {
	multiProj := &surface.RecipeProjection{
		RecipeID: "recipe.multiparadigm", Name: "Multi-Paradigm", RouteKind: "capability",
		Steps: []surface.ProjectedStep{
			{StepID: "s1", Paradigm: "react", Goal: "React step"},
			{StepID: "s2", Paradigm: "planner", Goal: "Planner step"},
			{StepID: "s3", Paradigm: "htn", Goal: "HTN step"},
			{StepID: "s4", Paradigm: "reflection", Goal: "Reflection step"},
			{StepID: "s5", Paradigm: "blackboard", Goal: "Blackboard step"},
			{StepID: "s6", Paradigm: "chainer", Goal: "Chainer step"},
			{StepID: "s7", Paradigm: "pipeline", Goal: "Pipeline step"},
			{StepID: "s8", Paradigm: "rewoo", Goal: "Rewoo step"},
			{StepID: "s9", Paradigm: "goalcon", Goal: "Goalcon step"},
			{StepID: "s10", Paradigm: "euclo", Goal: "Euclo step"},
		},
	}

	router := NewEucloEventRouter()
	feedRecipe(t, router, multiProj)

	for i, s := range multiProj.Steps {
		feedStepStarted(t, router, s.StepID, s.Paradigm, i, len(multiProj.Steps))
		feedStepCompleted(t, router, s.StepID, s.Paradigm, i, len(multiProj.Steps), true, int64((i+1)*10))
	}

	checkSnapshot(t, router, surface.MacroExecute, 10)

	// Theme has distinct glyphs for all 10 paradigms.
	th := theme.Default()
	seen := make(map[string]bool)
	for _, s := range multiProj.Steps {
		g := th.ParadigmGlyph(surface.Paradigm(s.Paradigm))
		if g == "" {
			t.Errorf("empty glyph for paradigm %q", s.Paradigm)
		}
		if seen[g] {
			t.Errorf("duplicate glyph %q for paradigm %q", g, s.Paradigm)
		}
		seen[g] = true
	}

	// Stepper render shows all 10 steps.
	stepper := NewStepper(router.Snapshot().Recipe, router.Snapshot().StepRuntime, router.Snapshot().Macro)
	rendered := stepper.Render(theme.Default())
	for _, s := range multiProj.Steps {
		if !strings.Contains(rendered, s.Paradigm) {
			t.Errorf("stepper missing paradigm %q in render:\n%s", s.Paradigm, rendered)
		}
	}
}

// ── Failed step ────────────────────────────────────────────────────────────

func TestIntegrationFailedStep(t *testing.T) {
	router := NewEucloEventRouter()
	feedRecipe(t, router, recipeHelpers.linear)

	feedStepStarted(t, router, "step.1", "react", 0, 3)
	feedStepCompleted(t, router, "step.1", "react", 0, 3, false, 100)

	snap := router.Snapshot()
	if snap.StepRuntime["step.1"].Status != surface.StepFailed {
		t.Errorf("step.1 status = %v, want failed", snap.StepRuntime["step.1"].Status)
	}

	// Recipe pane shows failure indication.
	pane := NewRecipePane(router, theme.Default())
	v := pane.View()
	if !strings.Contains(v, "✗") {
		t.Errorf("expected failure glyph in pane view, got:\n%s", v)
	}
	if !strings.Contains(v, "100ms") {
		t.Errorf("expected 100ms duration in pane view:\n%s", v)
	}
}

// ── Full lifecycle with verify ─────────────────────────────────────────────

func TestIntegrationFullLifecycle(t *testing.T) {
	router := NewEucloEventRouter()

	router.ApplyExecutionEvent(ExecutionEvent{Type: reporting.EventTypeIntakeComplete})
	checkSnapshot(t, router, surface.MacroIntake, 0)

	router.ApplyExecutionEvent(ExecutionEvent{Type: reporting.EventTypeRouteSelected})
	checkSnapshot(t, router, surface.MacroRoute, 0)

	feedRecipe(t, router, recipeHelpers.linear)
	checkSnapshot(t, router, surface.MacroExecute, 0)

	for i, s := range recipeHelpers.linear.Steps {
		feedStepStarted(t, router, s.StepID, s.Paradigm, i, len(recipeHelpers.linear.Steps))
		feedStepCompleted(t, router, s.StepID, s.Paradigm, i, len(recipeHelpers.linear.Steps), true, int64((i+1)*10))
	}

	router.ApplyExecutionEvent(ExecutionEvent{Type: reporting.EventTypeVerifyStarted})
	checkSnapshot(t, router, surface.MacroVerify, 3)

	router.ApplyExecutionEvent(ExecutionEvent{Type: reporting.EventTypeExecutionComplete})
	snap := router.Snapshot()
	if snap.Macro != surface.MacroDone {
		t.Errorf("final macro = %v, want done", snap.Macro)
	}
}
