package thoughtrecipes

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
)

func TestNewRecipeState(t *testing.T) {
	task := &core.Task{
		Instruction: "test instruction",
		Type:        core.TaskTypeAnalysis,
		Context: map[string]any{
			"workspace": "/test/workspace",
		},
	}

	state := NewRecipeState(task)

	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if state.Base == nil {
		t.Error("expected non-nil Base")
	}
	if state.Captures == nil {
		t.Error("expected non-nil Captures map")
	}
	if state.StepResults == nil {
		t.Error("expected non-nil StepResults map")
	}

	// Check task metadata was set
	if val, ok := state.Base.Get("task.instruction"); !ok || val != "test instruction" {
		t.Errorf("expected task.instruction = 'test instruction', got %v", val)
	}

	// Check context values
	if val, ok := state.Base.Get("task.context.workspace"); !ok || val != "/test/workspace" {
		t.Errorf("expected task.context.workspace = '/test/workspace', got %v", val)
	}
}

func TestNewRecipeStateNilTask(t *testing.T) {
	state := NewRecipeState(nil)

	if state == nil {
		t.Fatal("expected non-nil state")
	}

	// Should not panic
	_, _ = state.Base.Get("any.key")
}

func TestRecipeStateCapture(t *testing.T) {
	state := NewRecipeState(nil)
	resolver := NewAliasResolver(nil)

	// Capture a standard alias
	err := state.Capture("explore_findings", "Found 5 files", resolver)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Check it was stored in Captures
	if val, ok := state.Captures["explore_findings"]; !ok || val != "Found 5 files" {
		t.Errorf("expected Captures['explore_findings'] = 'Found 5 files', got %v", val)
	}

	// Check it was also written to Base under resolved state key
	if val, ok := state.Base.Get("pipeline.explore"); !ok || val != "Found 5 files" {
		t.Errorf("expected Base['pipeline.explore'] = 'Found 5 files', got %v", val)
	}
}

func TestRecipeStateCaptureCustomAlias(t *testing.T) {
	state := NewRecipeState(nil)
	resolver := NewAliasResolver(map[string]string{
		"my_custom": "custom.state.key",
	})

	err := state.Capture("my_custom", "custom value", resolver)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Check Captures
	if val, ok := state.Captures["my_custom"]; !ok || val != "custom value" {
		t.Errorf("expected Captures['my_custom'] = 'custom value', got %v", val)
	}

	// Check Base uses resolved key
	if val, ok := state.Base.Get("custom.state.key"); !ok || val != "custom value" {
		t.Errorf("expected Base['custom.state.key'] = 'custom value', got %v", val)
	}
}

func TestRecipeStateCaptureUnknownAlias(t *testing.T) {
	state := NewRecipeState(nil)
	resolver := NewAliasResolver(nil)

	// Unknown alias should be stored under the alias name itself
	err := state.Capture("unknown_alias", "value", resolver)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Should be in Captures under the alias name
	if val, ok := state.Captures["unknown_alias"]; !ok || val != "value" {
		t.Errorf("expected Captures['unknown_alias'] = 'value', got %v", val)
	}

	// Should be in Base under the alias name (since resolution failed)
	if val, ok := state.Base.Get("unknown_alias"); !ok || val != "value" {
		t.Errorf("expected Base['unknown_alias'] = 'value', got %v", val)
	}
}

func TestRecipeStateCaptureNilState(t *testing.T) {
	var state *RecipeState
	resolver := NewAliasResolver(nil)

	// Should not panic
	err := state.Capture("any", "value", resolver)
	if err != nil {
		t.Errorf("expected no error for nil state, got %v", err)
	}
}

func TestRecipeStateGetCapture(t *testing.T) {
	state := NewRecipeState(nil)
	resolver := NewAliasResolver(nil)

	// Capture something
	state.Capture("test_alias", "test value", resolver)

	// Retrieve it
	val, ok := state.GetCapture("test_alias")
	if !ok {
		t.Error("expected to find captured value")
	}
	if val != "test value" {
		t.Errorf("expected 'test value', got %v", val)
	}

	// Non-existent alias
	_, ok = state.GetCapture("non_existent")
	if ok {
		t.Error("expected not to find non-existent alias")
	}
}

func TestRecipeStateGetCaptureNilState(t *testing.T) {
	var state *RecipeState

	_, ok := state.GetCapture("any")
	if ok {
		t.Error("expected GetCapture on nil state to return false")
	}
}

func TestRecipeStateAddStepResult(t *testing.T) {
	state := NewRecipeState(nil)

	result := &StepResult{
		StepID:  "step1",
		Success: true,
		Artifacts: []euclotypes.Artifact{
			{ID: "art1", Kind: euclotypes.ArtifactKindExplore},
		},
	}

	state.AddStepResult(result)

	// Check it was added
	stored, ok := state.StepResults["step1"]
	if !ok {
		t.Error("expected step result to be stored")
	}
	if !stored.Success {
		t.Error("expected stored result to be successful")
	}

	// Check artifacts were accumulated
	if len(state.Artifacts) != 1 {
		t.Errorf("expected 1 artifact accumulated, got %d", len(state.Artifacts))
	}
}

func TestRecipeStateAddStepResultNilState(t *testing.T) {
	var state *RecipeState

	// Should not panic
	state.AddStepResult(&StepResult{StepID: "step1"})
}

func TestRecipeStateBuildSharingContextCarryForward(t *testing.T) {
	state := NewRecipeState(nil)
	resolver := NewAliasResolver(nil)

	// Capture some values
	state.Capture("explore_findings", "exploration data", resolver)
	state.Capture("analysis_result", "analysis data", resolver)

	step := ExecutionStep{
		ID:      "step2",
		Inherit: []string{"explore_findings"},
	}

	// With carry_forward, all captures are inherited
	ctx := state.BuildSharingContext(step, SharingModeCarryForward, resolver)

	// Both captures should be present
	if len(ctx) != 2 {
		t.Errorf("expected 2 context entries, got %d", len(ctx))
	}
	if ctx["explore_findings"] != "exploration data" {
		t.Errorf("expected explore_findings in context")
	}
	if ctx["analysis_result"] != "analysis data" {
		t.Errorf("expected analysis_result in context")
	}
}

func TestRecipeStateBuildSharingContextExplicit(t *testing.T) {
	state := NewRecipeState(nil)
	resolver := NewAliasResolver(nil)

	// Capture some values
	state.Capture("explore_findings", "exploration data", resolver)
	state.Capture("analysis_result", "analysis data", resolver)

	step := ExecutionStep{
		ID:      "step2",
		Inherit: []string{"explore_findings"},
	}

	// With explicit, only inherited captures are visible
	ctx := state.BuildSharingContext(step, SharingModeExplicit, resolver)

	if len(ctx) != 1 {
		t.Errorf("expected 1 context entry, got %d: %v", len(ctx), ctx)
	}
	if ctx["explore_findings"] != "exploration data" {
		t.Errorf("expected explore_findings in context")
	}
	if _, ok := ctx["analysis_result"]; ok {
		t.Error("did not expect analysis_result in explicit context")
	}
}

func TestRecipeStateBuildSharingContextIsolated(t *testing.T) {
	state := NewRecipeState(nil)
	resolver := NewAliasResolver(nil)

	// Capture some values
	state.Capture("explore_findings", "exploration data", resolver)

	step := ExecutionStep{
		ID:      "step2",
		Inherit: []string{}, // No inherits declared
	}

	// With isolated, step only sees what it declares
	ctx := state.BuildSharingContext(step, SharingModeIsolated, resolver)

	if len(ctx) != 0 {
		t.Errorf("expected 0 context entries for isolated with no inherits, got %d", len(ctx))
	}
}

func TestRecipeStateBuildSharingContextNilState(t *testing.T) {
	var state *RecipeState
	resolver := NewAliasResolver(nil)
	step := ExecutionStep{ID: "step1"}

	ctx := state.BuildSharingContext(step, SharingModeCarryForward, resolver)

	if ctx == nil {
		t.Error("expected non-nil context map")
	}
	if len(ctx) != 0 {
		t.Errorf("expected empty context for nil state, got %v", ctx)
	}
}

func TestRecipeStateToRecipeResult(t *testing.T) {
	state := NewRecipeState(nil)
	resolver := NewAliasResolver(nil)

	// Add captures
	state.Capture("result1", "value1", resolver)
	state.Capture("result2", "value2", resolver)

	// Add step results
	state.AddStepResult(&StepResult{StepID: "step1", Success: true})
	state.AddStepResult(&StepResult{StepID: "step2", Success: true})

	warnings := []string{"warning1", "warning2"}
	result := state.ToRecipeResult(warnings)

	if result == nil {
		t.Fatal("expected non-nil RecipeResult")
	}

	if !result.Success {
		t.Error("expected Success to be true (has captures)")
	}

	if len(result.FinalCaptures) != 2 {
		t.Errorf("expected 2 final captures, got %d", len(result.FinalCaptures))
	}

	if len(result.StepResults) != 2 {
		t.Errorf("expected 2 step results, got %d", len(result.StepResults))
	}

	if len(result.Warnings) != 2 {
		t.Errorf("expected 2 warnings, got %d", len(result.Warnings))
	}
}

func TestRecipeStateToRecipeResultEmpty(t *testing.T) {
	state := NewRecipeState(nil)
	warnings := []string{"a warning"}

	result := state.ToRecipeResult(warnings)

	if result.Success {
		t.Error("expected Success to be false (no captures)")
	}

	if len(result.FinalCaptures) != 0 {
		t.Errorf("expected 0 captures, got %d", len(result.FinalCaptures))
	}

	if len(result.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(result.Warnings))
	}
}

func TestRecipeStateToRecipeResultNilState(t *testing.T) {
	var state *RecipeState
	warnings := []string{"warning"}

	result := state.ToRecipeResult(warnings)

	if result == nil {
		t.Fatal("expected non-nil RecipeResult")
	}

	if result.Success {
		t.Error("expected Success to be false (nil state)")
	}

	if result.FinalCaptures == nil {
		t.Error("expected non-nil FinalCaptures map")
	}

	if len(result.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(result.Warnings))
	}
}

func TestSharingModeConstants(t *testing.T) {
	if SharingModeCarryForward != "carry_forward" {
		t.Errorf("unexpected SharingModeCarryForward value: %q", SharingModeCarryForward)
	}
	if SharingModeIsolated != "isolated" {
		t.Errorf("unexpected SharingModeIsolated value: %q", SharingModeIsolated)
	}
	if SharingModeExplicit != "explicit" {
		t.Errorf("unexpected SharingModeExplicit value: %q", SharingModeExplicit)
	}
}

func TestEnrichmentSourceConstants(t *testing.T) {
	if EnrichmentAST != "ast" {
		t.Errorf("unexpected EnrichmentAST value: %q", EnrichmentAST)
	}
	if EnrichmentArchaeology != "archaeology" {
		t.Errorf("unexpected EnrichmentArchaeology value: %q", EnrichmentArchaeology)
	}
	if EnrichmentBKC != "bkc" {
		t.Errorf("unexpected EnrichmentBKC value: %q", EnrichmentBKC)
	}
}

func TestExecutionStepDefaults(t *testing.T) {
	step := ExecutionStep{
		ID: "step1",
		Parent: ExecutionStepAgent{
			Paradigm: "react",
			Prompt:   "Analyze this",
		},
	}

	if step.ID != "step1" {
		t.Errorf("expected ID 'step1', got %q", step.ID)
	}
	if step.Parent.Paradigm != "react" {
		t.Errorf("expected paradigm 'react', got %q", step.Parent.Paradigm)
	}
	if step.Parent.Prompt != "Analyze this" {
		t.Errorf("expected prompt 'Analyze this', got %q", step.Parent.Prompt)
	}
	if step.Child != nil {
		t.Error("expected nil Child")
	}
	if step.Fallback != nil {
		t.Error("expected nil Fallback")
	}
}

func TestExecutionPlanDefaults(t *testing.T) {
	plan := ExecutionPlan{
		Name:        "test-plan",
		Description: "A test plan",
		Steps:       []ExecutionStep{},
		Warnings:    []string{"test warning"},
	}

	if plan.Name != "test-plan" {
		t.Errorf("unexpected name: %q", plan.Name)
	}
	if len(plan.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(plan.Warnings))
	}
}

func TestStepResultStructure(t *testing.T) {
	result := &StepResult{
		StepID:  "step1",
		Success: true,
		Error:   "",
		Captures: map[string]interface{}{
			"result": "value",
		},
		Artifacts: []euclotypes.Artifact{
			{ID: "art1", Kind: euclotypes.ArtifactKindExplore},
		},
	}

	if result.StepID != "step1" {
		t.Errorf("unexpected StepID: %q", result.StepID)
	}
	if !result.Success {
		t.Error("expected Success to be true")
	}
	if len(result.Captures) != 1 {
		t.Errorf("expected 1 capture, got %d", len(result.Captures))
	}
	if len(result.Artifacts) != 1 {
		t.Errorf("expected 1 artifact, got %d", len(result.Artifacts))
	}
}
