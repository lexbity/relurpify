package recipe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecipeLoaderLoadFromBytes(t *testing.T) {
	loader := NewLoader()

	yamlData := []byte(`
id: test-recipe
name: Test Recipe
steps:
  - id: step1
    type: llm
`)

	recipe, err := loader.LoadFromBytes(yamlData)
	if err != nil {
		t.Fatalf("LoadFromBytes failed: %v", err)
	}

	if recipe.ID != "test-recipe" {
		t.Errorf("Expected ID test-recipe, got %s", recipe.ID)
	}

	if recipe.Name != "Test Recipe" {
		t.Errorf("Expected Name Test Recipe, got %s", recipe.Name)
	}

	if len(recipe.Steps) != 1 {
		t.Errorf("Expected 1 step, got %d", len(recipe.Steps))
	}

	if recipe.Steps[0].ID != "step1" {
		t.Errorf("Expected step ID step1, got %s", recipe.Steps[0].ID)
	}

	if recipe.Steps[0].Type != "llm" {
		t.Errorf("Expected step type llm, got %s", recipe.Steps[0].Type)
	}
}

func TestRecipeLoaderInvalidYAML(t *testing.T) {
	loader := NewLoader()

	invalidYAML := []byte(`
id: test-recipe
name: Test Recipe
steps:
  - id: step1
    type: llm
    invalid_yaml: [unclosed
`)

	_, err := loader.LoadFromBytes(invalidYAML)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestRecipeLoaderLoadFromFile(t *testing.T) {
	loader := NewLoader()

	// Create a temporary directory
	tmpDir := t.TempDir()

	// Create a temporary recipe file
	recipePath := filepath.Join(tmpDir, "test-recipe.yaml")
	yamlContent := `
id: test-recipe
name: Test Recipe
steps:
  - id: step1
    type: llm
`
	err := os.WriteFile(recipePath, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test recipe file: %v", err)
	}

	// Load the recipe from file
	recipe, err := loader.LoadFromFile(recipePath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if recipe.ID != "test-recipe" {
		t.Errorf("Expected ID test-recipe, got %s", recipe.ID)
	}

	if recipe.Name != "Test Recipe" {
		t.Errorf("Expected Name Test Recipe, got %s", recipe.Name)
	}
}

func TestRecipeLoaderFileNotFound(t *testing.T) {
	loader := NewLoader()

	_, err := loader.LoadFromFile("/nonexistent/path/recipe.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestRecipeLoaderInvalidRecipeInFile(t *testing.T) {
	loader := NewLoader()

	// Create a temporary directory
	tmpDir := t.TempDir()

	// Create a temporary recipe file with invalid recipe (missing ID)
	recipePath := filepath.Join(tmpDir, "invalid-recipe.yaml")
	yamlContent := `
name: Test Recipe
steps:
  - id: step1
    type: llm
`
	err := os.WriteFile(recipePath, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test recipe file: %v", err)
	}

	_, err = loader.LoadFromFile(recipePath)
	if err == nil {
		t.Error("Expected validation error for invalid recipe")
	}
}

func TestRecipeLoaderLoadsWorkspaceExamples(t *testing.T) {
	loader := NewLoader()

	files := []string{
		filepath.Join("..", "..", "..", "relurpify_cfg", "recipes", "simple-react.yaml"),
		filepath.Join("..", "..", "..", "relurpify_cfg", "recipes", "multi-step-capture.yaml"),
		filepath.Join("..", "..", "..", "relurpify_cfg", "recipes", "with-fallback.yaml"),
		filepath.Join("..", "..", "..", "relurpify_cfg", "recipes", "global-restricted.yaml"),
		filepath.Join("..", "..", "..", "relurpify_cfg", "recipes", "htn-with-child.yaml"),
	}

	for _, path := range files {
		recipe, err := loader.LoadFromFile(path)
		if err != nil {
			t.Fatalf("LoadFromFile(%s) failed: %v", path, err)
		}
		if recipe.EffectiveName() == "" {
			t.Fatalf("recipe from %s has no effective name", path)
		}
		if len(recipe.StepList()) == 0 {
			t.Fatalf("recipe from %s has no steps", path)
		}
	}
}
