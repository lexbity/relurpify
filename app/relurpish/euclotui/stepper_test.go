package euclotui

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

func TestStepperIdle(t *testing.T) {
	s := NewStepper(nil, nil, surface.MacroIdle)
	r := s.Render(theme.Default())
	if r != "" {
		t.Errorf("idle render should be empty, got: %q", r)
	}
}

func TestStepperMacroRailOnly(t *testing.T) {
	s := NewStepper(nil, nil, surface.MacroExecute)
	r := s.Render(theme.Default())
	if r == "" {
		t.Fatal("expected non-empty render for Execute phase")
	}
	if !strings.Contains(r, "intake") {
		t.Errorf("expected intake in rail, got: %s", r)
	}
	if !strings.Contains(r, "execute") {
		t.Errorf("expected execute in rail, got: %s", r)
	}
}

func TestStepperWithRecipeSteps(t *testing.T) {
	proj := &surface.RecipeProjection{
		RecipeID: "recipe.test",
		Name:     "Test Recipe",
		Steps: []surface.ProjectedStep{
			{StepID: "step.1", Paradigm: "goalcon", Goal: "Analyze input"},
			{StepID: "step.2", Paradigm: "react", Goal: "Process request"},
			{StepID: "step.3", Paradigm: "euclo", Goal: "Verify result"},
		},
	}
	runtime := map[string]surface.StepRuntime{
		"step.1": {StepID: "step.1", Status: surface.StepDone, Index: 0, Total: 3, Paradigm: "goalcon"},
		"step.2": {StepID: "step.2", Status: surface.StepActive, Index: 1, Total: 3, Paradigm: "react"},
	}
	s := NewStepper(proj, runtime, surface.MacroExecute)
	r := s.Render(theme.Default())
	if r == "" {
		t.Fatal("expected non-empty render")
	}
	if !strings.Contains(r, "Analyze input") {
		t.Errorf("expected step.1 goal in render, got: %s", r)
	}
	if !strings.Contains(r, "Process request") {
		t.Errorf("expected step.2 goal in render, got: %s", r)
	}
	if !strings.Contains(r, "Verify result") {
		t.Errorf("expected step.3 goal in render, got: %s", r)
	}
	if !strings.Contains(r, "(1/3)") {
		t.Errorf("expected (1/3) for step.2 (active), got: %s", r)
	}
}

func TestStepperParallelGroup(t *testing.T) {
	proj := &surface.RecipeProjection{
		RecipeID: "recipe.parallel",
		Name:     "Parallel Recipe",
		Steps: []surface.ProjectedStep{
			{StepID: "intro", Paradigm: "goalcon", Goal: "Intro"},
			{StepID: "left", Paradigm: "react", Goal: "Left branch"},
			{StepID: "right", Paradigm: "react", Goal: "Right branch"},
			{StepID: "merge", Paradigm: "goalcon", Goal: "Merge results"},
		},
		Groups: []surface.ProjectedGroup{
			{GroupID: "fanout", Kind: "parallel", MemberStepIDs: []string{"left", "right"}, Merge: "all"},
		},
	}
	runtime := map[string]surface.StepRuntime{
		"intro": {StepID: "intro", Status: surface.StepDone, Index: 0, Total: 4, Paradigm: "goalcon"},
		"left":  {StepID: "left", Status: surface.StepActive, Index: 1, Total: 4, Paradigm: "react"},
		"right": {StepID: "right", Status: surface.StepActive, Index: 2, Total: 4, Paradigm: "react"},
	}
	s := NewStepper(proj, runtime, surface.MacroExecute)
	r := s.Render(theme.Default())
	if !strings.Contains(r, "Left branch") || !strings.Contains(r, "Right branch") {
		t.Errorf("parallel steps should be visible, got: %s", r)
	}
}

func TestStepperConditionalWithSkippedBranch(t *testing.T) {
	proj := &surface.RecipeProjection{
		RecipeID: "recipe.conditional",
		Name:     "Conditional Recipe",
		Steps: []surface.ProjectedStep{
			{StepID: "preamble", Paradigm: "euclo", Goal: "Setup"},
			{StepID: "if.branch", Paradigm: "goalcon", Goal: "Primary path"},
			{StepID: "else.branch", Paradigm: "react", Goal: "Fallback path", Optional: true},
		},
		Groups: []surface.ProjectedGroup{
			{GroupID: "decision", Kind: "conditional", MemberStepIDs: []string{"if.branch", "else.branch"}, Condition: "thoughtrecipe.branch"},
		},
	}
	runtime := map[string]surface.StepRuntime{
		"preamble":   {StepID: "preamble", Status: surface.StepDone, Index: 0, Total: 3, Paradigm: "euclo"},
		"if.branch":  {StepID: "if.branch", Status: surface.StepDone, Index: 1, Total: 3, Paradigm: "goalcon"},
		"else.branch": {StepID: "else.branch", Status: surface.StepSkipped, Index: 2, Total: 3, Paradigm: "react"},
	}
	s := NewStepper(proj, runtime, surface.MacroVerify)
	r := s.Render(theme.Default())
	if !strings.Contains(r, "Primary path") {
		t.Errorf("primary branch should be visible, got: %s", r)
	}
	if !strings.Contains(r, "Fallback path") {
		t.Errorf("skipped fallback should still render, got: %s", r)
	}
}

func TestStepperFailedStep(t *testing.T) {
	proj := &surface.RecipeProjection{
		RecipeID: "recipe.fail",
		Name:     "Failing Recipe",
		Steps: []surface.ProjectedStep{
			{StepID: "step.1", Paradigm: "goalcon", Goal: "Will fail"},
		},
	}
	runtime := map[string]surface.StepRuntime{
		"step.1": {StepID: "step.1", Status: surface.StepFailed, Index: 0, Total: 1, Paradigm: "goalcon", DurationMs: 50, Err: "error"},
	}
	s := NewStepper(proj, runtime, surface.MacroDone)
	r := s.Render(theme.Default())
	if !strings.Contains(r, "Will fail") {
		t.Errorf("failed step should be visible, got: %s", r)
	}
}

func TestStepperNilSafe(t *testing.T) {
	var s *Stepper
	r := s.Render(theme.Default())
	if r != "" {
		t.Errorf("nil stepper should render empty, got: %q", r)
	}
}

func TestStepperNilTheme(t *testing.T) {
	s := NewStepper(nil, nil, surface.MacroExecute)
	r := s.Render(nil)
	if r != "" {
		t.Errorf("nil theme should render empty, got: %q", r)
	}
}

func TestStepperMacroRailLabels(t *testing.T) {
	tests := []struct {
		macro surface.MacroPhase
		want  string
	}{
		{surface.MacroIntake, "intake"},
		{surface.MacroRoute, "route"},
		{surface.MacroExecute, "execute"},
		{surface.MacroVerify, "verify"},
		{surface.MacroDone, "done"},
	}
	for _, tt := range tests {
		s := NewStepper(nil, nil, tt.macro)
		r := s.Render(theme.Default())
		if !strings.Contains(r, tt.want) {
			t.Errorf("macro=%v: expected %q in render, got: %s", tt.macro, tt.want, r)
		}
	}
}
