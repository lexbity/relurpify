package recipetemplates

import (
	"testing"

	recipepkg "codeburg.org/lexbit/relurpify/named/euclo/recipes"
)

func TestLoadAllParsesDebugTDDRepair(t *testing.T) {
	registry, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll returned error: %v", err)
	}

	recipe, ok := registry.Get("euclo.recipe.debug_tdd_repair")
	if !ok {
		t.Fatal("recipe 'euclo.recipe.debug_tdd_repair' not found in registry")
	}

	if recipe == nil {
		t.Fatal("recipe is nil")
	}

	if recipe.ID != "euclo.recipe.debug_tdd_repair" {
		t.Errorf("recipe ID = %q, want %q", recipe.ID, "euclo.recipe.debug_tdd_repair")
	}
}

func TestDebugTDDRepairHasThreeSteps(t *testing.T) {
	registry, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll returned error: %v", err)
	}

	recipe, ok := registry.Get("euclo.recipe.debug_tdd_repair")
	if !ok {
		t.Fatal("recipe 'euclo.recipe.debug_tdd_repair' not found in registry")
	}

	steps := recipe.StepList()
	if len(steps) != 3 {
		t.Fatalf("recipe has %d steps, want 3", len(steps))
	}

	expectedStepIDs := []string{"locate", "repair", "verify"}
	for i, step := range steps {
		if step.ID != expectedStepIDs[i] {
			t.Errorf("step %d ID = %q, want %q", i, step.ID, expectedStepIDs[i])
		}
	}
}

func TestDebugTDDRepairRepairStepHasHITL(t *testing.T) {
	registry, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll returned error: %v", err)
	}

	recipe, ok := registry.Get("euclo.recipe.debug_tdd_repair")
	if !ok {
		t.Fatal("recipe 'euclo.recipe.debug_tdd_repair' not found in registry")
	}

	steps := recipe.StepList()
	var repairStep *recipepkg.RecipeStep
	for _, step := range steps {
		if step.ID == "repair" {
			repairStep = &step
			break
		}
	}

	if repairStep == nil {
		t.Fatal("repair step not found")
	}

	if repairStep.HITL != "ask" {
		t.Errorf("repair step HITL = %q, want %q", repairStep.HITL, "ask")
	}

	if repairStep.Mutation != "required" {
		t.Errorf("repair step Mutation = %q, want %q", repairStep.Mutation, "required")
	}
}

func TestDebugTDDRepairRepairHasFallback(t *testing.T) {
	registry, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll returned error: %v", err)
	}

	recipe, ok := registry.Get("euclo.recipe.debug_tdd_repair")
	if !ok {
		t.Fatal("recipe 'euclo.recipe.debug_tdd_repair' not found in registry")
	}

	steps := recipe.StepList()
	var repairStep *recipepkg.RecipeStep
	for _, step := range steps {
		if step.ID == "repair" {
			repairStep = &step
			break
		}
	}

	if repairStep == nil {
		t.Fatal("repair step not found")
	}

	if repairStep.Fallback == nil {
		t.Fatal("repair step has no fallback, want non-nil fallback")
	}
}

func TestLoadAllParsesAllTemplates(t *testing.T) {
	registry, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll returned error: %v", err)
	}

	expectedIDs := []string{
		"euclo.recipe.debug_tdd_repair",
		"euclo.recipe.code_review",
		"euclo.recipe.investigation",
		"euclo.recipe.extract_func",
		"euclo.recipe.test_synthesis",
		"euclo.recipe.dep_upgrade",
	}

	for _, id := range expectedIDs {
		recipe, ok := registry.Get(id)
		if !ok {
			t.Errorf("recipe %q not found in registry", id)
		}
		if recipe == nil {
			t.Errorf("recipe %q is nil", id)
		}
	}
}

func TestInvestigationHasTwoSteps(t *testing.T) {
	registry, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll returned error: %v", err)
	}

	recipe, ok := registry.Get("euclo.recipe.investigation")
	if !ok {
		t.Fatal("recipe 'euclo.recipe.investigation' not found in registry")
	}

	steps := recipe.StepList()
	if len(steps) != 2 {
		t.Fatalf("recipe has %d steps, want 2", len(steps))
	}

	expectedStepIDs := []string{"trace", "report"}
	for i, step := range steps {
		if step.ID != expectedStepIDs[i] {
			t.Errorf("step %d ID = %q, want %q", i, step.ID, expectedStepIDs[i])
		}
	}
}

func TestCodeReviewRecipeIsSingleStep(t *testing.T) {
	registry, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll returned error: %v", err)
	}

	recipe, ok := registry.Get("euclo.recipe.code_review")
	if !ok {
		t.Fatal("recipe 'euclo.recipe.code_review' not found in registry")
	}

	steps := recipe.StepList()
	if len(steps) != 1 {
		t.Fatalf("recipe has %d steps, want 1", len(steps))
	}

	step := steps[0]
	if step.Mutation != "none" {
		t.Errorf("step Mutation = %q, want %q", step.Mutation, "none")
	}

	if step.HITL != "never" {
		t.Errorf("step HITL = %q, want %q", step.HITL, "never")
	}
}

func TestDepUpgradeHasHITLOnMigrate(t *testing.T) {
	registry, err := LoadAll()
	if err != nil {
		t.Fatalf("LoadAll returned error: %v", err)
	}

	recipe, ok := registry.Get("euclo.recipe.dep_upgrade")
	if !ok {
		t.Fatal("recipe 'euclo.recipe.dep_upgrade' not found in registry")
	}

	steps := recipe.StepList()
	var migrateStep *recipepkg.RecipeStep
	for _, step := range steps {
		if step.ID == "migrate" {
			migrateStep = &step
			break
		}
	}

	if migrateStep == nil {
		t.Fatal("migrate step not found")
	}

	if migrateStep.HITL != "ask" {
		t.Errorf("migrate step HITL = %q, want %q", migrateStep.HITL, "ask")
	}
}
