package recipe

import (
	"testing"
)

func TestRecipeSchemaValidation(t *testing.T) {
	recipe := &ThoughtRecipe{
		ID:   "test-recipe",
		Name: "Test Recipe",
		Steps: []RecipeStep{
			{
				ID:   "step1",
				Type: "llm",
			},
		},
	}

	err := recipe.Validate()
	if err != nil {
		t.Errorf("Expected valid recipe to pass validation, got error: %v", err)
	}
}

func TestRecipeSchemaMissingID(t *testing.T) {
	recipe := &ThoughtRecipe{
		Name: "Test Recipe",
		Steps: []RecipeStep{
			{
				ID:   "step1",
				Type: "llm",
			},
		},
	}

	err := recipe.Validate()
	if err == nil {
		t.Error("Expected validation error for missing ID")
	}

	if err.Error() != "recipe missing required field: id" {
		t.Errorf("Expected 'recipe missing required field: id' error, got: %v", err)
	}
}

func TestRecipeSchemaInvalidStepType(t *testing.T) {
	recipe := &ThoughtRecipe{
		ID:   "test-recipe",
		Name: "Test Recipe",
		Steps: []RecipeStep{
			{
				ID:   "step1",
				Type: "invalid_type",
			},
		},
	}

	err := recipe.Validate()
	if err == nil {
		t.Error("Expected validation error for invalid step type")
	}
}

func TestRecipeSchemaMissingName(t *testing.T) {
	recipe := &ThoughtRecipe{
		ID: "test-recipe",
		Steps: []RecipeStep{
			{
				ID:   "step1",
				Type: "llm",
			},
		},
	}

	err := recipe.Validate()
	if err == nil {
		t.Error("Expected validation error for missing name")
	}
}

func TestRecipeSchemaNoSteps(t *testing.T) {
	recipe := &ThoughtRecipe{
		ID:    "test-recipe",
		Name:  "Test Recipe",
		Steps: []RecipeStep{},
	}

	err := recipe.Validate()
	if err == nil {
		t.Error("Expected validation error for no steps")
	}
}

func TestRecipeSchemaDuplicateStepID(t *testing.T) {
	recipe := &ThoughtRecipe{
		ID:   "test-recipe",
		Name: "Test Recipe",
		Steps: []RecipeStep{
			{
				ID:   "step1",
				Type: "llm",
			},
			{
				ID:   "step1",
				Type: "retrieve",
			},
		},
	}

	err := recipe.Validate()
	if err == nil {
		t.Error("Expected validation error for duplicate step ID")
	}
}

func TestRecipeSchemaMissingStepID(t *testing.T) {
	recipe := &ThoughtRecipe{
		ID:   "test-recipe",
		Name: "Test Recipe",
		Steps: []RecipeStep{
			{
				Type: "llm",
			},
		},
	}

	err := recipe.Validate()
	if err == nil {
		t.Error("Expected validation error for missing step ID")
	}
}

func TestRecipeSchemaMissingStepType(t *testing.T) {
	recipe := &ThoughtRecipe{
		ID:   "test-recipe",
		Name: "Test Recipe",
		Steps: []RecipeStep{
			{
				ID: "step1",
			},
		},
	}

	err := recipe.Validate()
	if err == nil {
		t.Error("Expected validation error for missing step type")
	}
}

func TestRecipeSchemaInvalidDependency(t *testing.T) {
	recipe := &ThoughtRecipe{
		ID:   "test-recipe",
		Name: "Test Recipe",
		Steps: []RecipeStep{
			{
				ID:           "step1",
				Type:         "llm",
				Dependencies: []string{"non_existent_step"},
			},
		},
	}

	err := recipe.Validate()
	if err == nil {
		t.Error("Expected validation error for invalid dependency")
	}
}

func TestRecipeSchemaCapabilityStepValidation(t *testing.T) {
	recipe := &ThoughtRecipe{
		APIVersion: "euclo.v1",
		Kind:       "thought-recipe",
		Metadata: RecipeMetadata{
			Name: "Capability Recipe",
		},
		Sequence: RecipeSequence{
			Steps: []RecipeStep{
				{
					ID:           "step1",
					CapabilityID: "euclo:cap.ast_query",
				},
			},
		},
	}

	if err := recipe.Validate(); err != nil {
		t.Fatalf("expected capability step recipe to validate, got %v", err)
	}
}

func TestRecipeSchemaCapabilityStepAndParadigmConflict(t *testing.T) {
	recipe := &ThoughtRecipe{
		APIVersion: "euclo.v1",
		Kind:       "thought-recipe",
		Metadata: RecipeMetadata{
			Name: "Capability Recipe",
		},
		Sequence: RecipeSequence{
			Steps: []RecipeStep{
				{
					ID:           "step1",
					CapabilityID: "euclo:cap.ast_query",
					Parent: RecipeStepAgent{
						Paradigm: "react",
					},
				},
			},
		},
	}

	err := recipe.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if got := err.Error(); got != "step step1: capability_id and parent.paradigm are mutually exclusive" {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
