package euclotui

import (
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

func TestEucloEventRouterMacroPhaseTransitions(t *testing.T) {
	router := NewEucloEventRouter()

	// Intake complete → MacroIntake
	router.ApplyExecutionEvent(ExecutionEvent{
		Type: reporting.EventTypeIntakeComplete,
	})
	snap := router.Snapshot()
	if snap.Macro != surface.MacroIntake {
		t.Errorf("after intake: Macro = %v, want %v", snap.Macro, surface.MacroIntake)
	}

	// Route selected → MacroRoute
	router.ApplyExecutionEvent(ExecutionEvent{
		Type: reporting.EventTypeRouteSelected,
	})
	snap = router.Snapshot()
	if snap.Macro != surface.MacroRoute {
		t.Errorf("after route: Macro = %v, want %v", snap.Macro, surface.MacroRoute)
	}

	// Recipe selected → MacroExecute + recipe populated
	proj := surface.RecipeProjection{
		RecipeID: "recipe.test",
		Name:     "Test Recipe",
		Steps: []surface.ProjectedStep{
			{StepID: "step.1", Paradigm: "goalcon"},
			{StepID: "step.2", Paradigm: "react"},
		},
	}
	router.ApplyExecutionEvent(ExecutionEvent{
		Type: reporting.EventTypeRecipeSelected,
		Payload: map[string]any{
			"recipe": proj,
		},
	})
	snap = router.Snapshot()
	if snap.Macro != surface.MacroExecute {
		t.Errorf("after recipe selected: Macro = %v, want %v", snap.Macro, surface.MacroExecute)
	}
	if snap.Recipe == nil {
		t.Fatal("Recipe should be non-nil after recipe.selected")
	}
	if snap.Recipe.RecipeID != "recipe.test" {
		t.Errorf("Recipe.RecipeID = %q, want %q", snap.Recipe.RecipeID, "recipe.test")
	}
	if len(snap.Recipe.Steps) != 2 {
		t.Errorf("Recipe steps = %d, want 2", len(snap.Recipe.Steps))
	}

	// Step started → StepRuntime active
	router.ApplyExecutionEvent(ExecutionEvent{
		Type:     reporting.EventTypeStepStartedEuclo,
		StepID:   "step.1",
		Paradigm: "goalcon",
		Index:    0,
		Total:    2,
	})
	snap = router.Snapshot()
	rt, ok := snap.StepRuntime["step.1"]
	if !ok {
		t.Fatal("expected step.1 in StepRuntime")
	}
	if rt.Status != surface.StepActive {
		t.Errorf("step.1 status = %v, want %v", rt.Status, surface.StepActive)
	}
	if rt.Index != 0 || rt.Total != 2 {
		t.Errorf("step.1 index/total = %d/%d, want 0/2", rt.Index, rt.Total)
	}

	// Step completed → StepRuntime done
	router.ApplyExecutionEvent(ExecutionEvent{
		Type:       reporting.EventTypeStepCompletedEuclo,
		StepID:     "step.1",
		Paradigm:   "goalcon",
		Index:      0,
		Total:      2,
		Success:    true,
		DurationMs: 150,
	})
	snap = router.Snapshot()
	rt, ok = snap.StepRuntime["step.1"]
	if !ok {
		t.Fatal("expected step.1 in StepRuntime after completion")
	}
	if rt.Status != surface.StepDone {
		t.Errorf("step.1 status = %v, want %v", rt.Status, surface.StepDone)
	}
	if rt.DurationMs != 150 {
		t.Errorf("step.1 DurationMs = %d, want 150", rt.DurationMs)
	}

	// Step failed
	router.ApplyExecutionEvent(ExecutionEvent{
		Type:     reporting.EventTypeStepStartedEuclo,
		StepID:   "step.2",
		Paradigm: "react",
		Index:    1,
		Total:    2,
	})
	router.ApplyExecutionEvent(ExecutionEvent{
		Type:       reporting.EventTypeStepCompletedEuclo,
		StepID:     "step.2",
		Paradigm:   "react",
		Index:      1,
		Total:      2,
		Success:    false,
		DurationMs: 50,
		Payload:    map[string]any{"error": "timeout"},
	})
	snap = router.Snapshot()
	rt, ok = snap.StepRuntime["step.2"]
	if !ok {
		t.Fatal("expected step.2 in StepRuntime")
	}
	if rt.Status != surface.StepFailed {
		t.Errorf("step.2 status = %v, want %v", rt.Status, surface.StepFailed)
	}
	if rt.Err != "timeout" {
		t.Errorf("step.2 error = %q, want %q", rt.Err, "timeout")
	}

	// Verify started → MacroVerify
	router.ApplyExecutionEvent(ExecutionEvent{
		Type: reporting.EventTypeVerifyStarted,
	})
	snap = router.Snapshot()
	if snap.Macro != surface.MacroVerify {
		t.Errorf("after verify started: Macro = %v, want %v", snap.Macro, surface.MacroVerify)
	}

	// Execution complete → MacroDone
	router.ApplyExecutionEvent(ExecutionEvent{
		Type: reporting.EventTypeExecutionComplete,
	})
	snap = router.Snapshot()
	if snap.Macro != surface.MacroDone {
		t.Errorf("after execution complete: Macro = %v, want %v", snap.Macro, surface.MacroDone)
	}

	// Final snapshot assertions
	if len(snap.StepRuntime) != 2 {
		t.Errorf("StepRuntime count = %d, want 2", len(snap.StepRuntime))
	}
}

func TestEucloEventRouterStepRuntimeDeepCopy(t *testing.T) {
	router := NewEucloEventRouter()

	router.ApplyExecutionEvent(ExecutionEvent{
		Type: reporting.EventTypeRecipeSelected,
		Payload: map[string]any{
			"recipe": surface.RecipeProjection{
				RecipeID: "recipe.copy",
			},
		},
	})
	router.ApplyExecutionEvent(ExecutionEvent{
		Type:     reporting.EventTypeStepStartedEuclo,
		StepID:   "step.1",
		Paradigm: "goalcon",
		Index:    0,
		Total:    1,
	})

	snap1 := router.Snapshot()
	// Mutate the router's internal state
	router.ApplyExecutionEvent(ExecutionEvent{
		Type:       reporting.EventTypeStepCompletedEuclo,
		StepID:     "step.1",
		Paradigm:   "goalcon",
		Index:      0,
		Total:      1,
		Success:    true,
		DurationMs: 100,
	})

	// snap1 should be unchanged (deep copy)
	if snap1.StepRuntime["step.1"].Status != surface.StepActive {
		t.Errorf("snap1 step.1 status = %v, want %v (should be frozen at Active)", snap1.StepRuntime["step.1"].Status, surface.StepActive)
	}

	snap2 := router.Snapshot()
	if snap2.StepRuntime["step.1"].Status != surface.StepDone {
		t.Errorf("snap2 step.1 status = %v, want %v", snap2.StepRuntime["step.1"].Status, surface.StepDone)
	}
}

func TestEucloEventRouterRecipeNilWhenNotSelected(t *testing.T) {
	router := NewEucloEventRouter()
	snap := router.Snapshot()
	if snap.Recipe != nil {
		t.Error("Recipe should be nil before recipe.selected event")
	}
}

func TestEucloEventRouterComplexStepRuntime(t *testing.T) {
	router := NewEucloEventRouter()

	proj := surface.RecipeProjection{
		RecipeID: "recipe.multi",
		Name:     "Multi-step",
		Steps: []surface.ProjectedStep{
			{StepID: "s1", Paradigm: "goalcon"},
			{StepID: "s2", Paradigm: "react"},
			{StepID: "s3", Paradigm: "euclo"},
		},
	}
	router.ApplyExecutionEvent(ExecutionEvent{
		Type:    reporting.EventTypeRecipeSelected,
		Payload: map[string]any{"recipe": proj},
	})
	router.ApplyExecutionEvent(ExecutionEvent{
		Type:     reporting.EventTypeStepStartedEuclo,
		StepID:   "s1",
		Paradigm: "goalcon",
		Index:    0,
		Total:    3,
	})
	router.ApplyExecutionEvent(ExecutionEvent{
		Type:       reporting.EventTypeStepCompletedEuclo,
		StepID:     "s1",
		Paradigm:   "goalcon",
		Index:      0,
		Total:      3,
		Success:    true,
		DurationMs: 50,
	})
	router.ApplyExecutionEvent(ExecutionEvent{
		Type:     reporting.EventTypeStepStartedEuclo,
		StepID:   "s2",
		Paradigm: "react",
		Index:    1,
		Total:    3,
	})
	// s3 never starts — test partial runtime state

	snap := router.Snapshot()
	if len(snap.StepRuntime) != 2 {
		t.Errorf("StepRuntime len = %d, want 2", len(snap.StepRuntime))
	}
	if snap.StepRuntime["s1"].Status != surface.StepDone {
		t.Errorf("s1 = %v, want done", snap.StepRuntime["s1"].Status)
	}
	if snap.StepRuntime["s2"].Status != surface.StepActive {
		t.Errorf("s2 = %v, want active", snap.StepRuntime["s2"].Status)
	}
	if _, exists := snap.StepRuntime["s3"]; exists {
		t.Errorf("s3 should not exist in StepRuntime (never started)")
	}
}

type fakeRecipeLookup struct {
	recipes map[string]*surface.RecipeProjection
}

func (l *fakeRecipeLookup) LookupRecipe(id string) (*surface.RecipeProjection, bool) {
	if l == nil || l.recipes == nil {
		return nil, false
	}
	r, ok := l.recipes[id]
	return r, ok
}

func TestEucloEventRouterResumeDataRoundTrip(t *testing.T) {
	router := NewEucloEventRouter()

	proj := surface.RecipeProjection{
		RecipeID: "recipe.resume",
		Name:     "Resume Test",
		Steps: []surface.ProjectedStep{
			{StepID: "s1", Paradigm: "goalcon", Goal: "First"},
			{StepID: "s2", Paradigm: "react", Goal: "Second"},
		},
	}
	router.ApplyExecutionEvent(ExecutionEvent{
		Type:    reporting.EventTypeRecipeSelected,
		Payload: map[string]any{"recipe": proj},
	})
	router.ApplyExecutionEvent(ExecutionEvent{
		Type:     reporting.EventTypeStepStartedEuclo,
		StepID:   "s1",
		Paradigm: "goalcon",
		Index:    0,
		Total:    2,
	})
	router.ApplyExecutionEvent(ExecutionEvent{
		Type:       reporting.EventTypeStepCompletedEuclo,
		StepID:     "s1",
		Paradigm:   "goalcon",
		Index:      0,
		Total:      2,
		Success:    true,
		DurationMs: 50,
	})
	router.ApplyExecutionEvent(ExecutionEvent{
		Type:     reporting.EventTypeStepStartedEuclo,
		StepID:   "s2",
		Paradigm: "react",
		Index:    1,
		Total:    2,
	})

	// Capture resume data (simulates persisting mid-run).
	data := router.ResumeData()
	if data.RecipeID != "recipe.resume" {
		t.Errorf("ResumeData RecipeID = %q, want %q", data.RecipeID, "recipe.resume")
	}
	if len(data.StepRuntime) != 2 {
		t.Errorf("ResumeData StepRuntime count = %d, want 2", len(data.StepRuntime))
	}
	if data.StepRuntime["s1"].Status != surface.StepDone {
		t.Errorf("s1 status in ResumeData = %v, want done", data.StepRuntime["s1"].Status)
	}
	if data.StepRuntime["s2"].Status != surface.StepActive {
		t.Errorf("s2 status in ResumeData = %v, want active", data.StepRuntime["s2"].Status)
	}

	// Simulate resume: create a new router and apply resume data.
	newRouter := NewEucloEventRouter()
	lookup := &fakeRecipeLookup{
		recipes: map[string]*surface.RecipeProjection{
			"recipe.resume": &proj,
		},
	}
	newRouter.ApplyResumeData(data, lookup)

	snap := newRouter.Snapshot()
	if snap.Recipe == nil {
		t.Fatal("resumed snapshot should have Recipe")
	}
	if snap.Recipe.RecipeID != "recipe.resume" {
		t.Errorf("resumed RecipeID = %q, want %q", snap.Recipe.RecipeID, "recipe.resume")
	}
	if snap.Recipe.Name != "Resume Test" {
		t.Errorf("resumed Name = %q, want %q", snap.Recipe.Name, "Resume Test")
	}
	if len(snap.Recipe.Steps) != 2 {
		t.Errorf("resumed steps = %d, want 2", len(snap.Recipe.Steps))
	}
	if snap.StepRuntime["s1"].Status != surface.StepDone {
		t.Errorf("resumed s1 status = %v, want done", snap.StepRuntime["s1"].Status)
	}
	if snap.StepRuntime["s2"].Status != surface.StepActive {
		t.Errorf("resumed s2 status = %v, want active", snap.StepRuntime["s2"].Status)
	}
}

func TestEucloEventRouterResumeDataEmptyRecipeDegrades(t *testing.T) {
	newRouter := NewEucloEventRouter()
	data := RecipeResumeData{
		RecipeID: "recipe.missing",
		Macro:    surface.MacroExecute,
	}

	// No lookup registered — ApplyResumeData should not crash and set macro.
	newRouter.ApplyResumeData(data, nil)
	snap := newRouter.Snapshot()
	if snap.Macro != surface.MacroExecute {
		t.Errorf("macro = %v, want %v", snap.Macro, surface.MacroExecute)
	}
	if snap.Recipe != nil {
		t.Error("Recipe should be nil when lookup returns nothing")
	}
}

func TestEucloEventRouterResumeDataRegistersStepStatusesOnly(t *testing.T) {
	newRouter := NewEucloEventRouter()
	data := RecipeResumeData{
		RecipeID: "recipe.steps-only",
		StepRuntime: map[string]surface.StepRuntime{
			"s1": {StepID: "s1", Status: surface.StepDone, Paradigm: "goalcon"},
			"s2": {StepID: "s2", Status: surface.StepFailed, Paradigm: "react", Err: "timeout"},
		},
		Macro: surface.MacroDone,
	}
	newRouter.ApplyResumeData(data, nil)
	snap := newRouter.Snapshot()
	if len(snap.StepRuntime) != 2 {
		t.Errorf("StepRuntime count = %d, want 2", len(snap.StepRuntime))
	}
	if snap.StepRuntime["s1"].Status != surface.StepDone {
		t.Errorf("s1 status = %v, want done", snap.StepRuntime["s1"].Status)
	}
	if snap.StepRuntime["s2"].Status != surface.StepFailed {
		t.Errorf("s2 status = %v, want failed", snap.StepRuntime["s2"].Status)
	}
	if snap.StepRuntime["s2"].Err != "timeout" {
		t.Errorf("s2 error = %q, want %q", snap.StepRuntime["s2"].Err, "timeout")
	}
	if snap.Macro != surface.MacroDone {
		t.Errorf("macro = %v, want %v", snap.Macro, surface.MacroDone)
	}
}
