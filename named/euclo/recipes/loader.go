package recipe

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Loader loads recipes from files or byte slices.
type Loader struct{}

// NewLoader creates a new recipe loader.
func NewLoader() *Loader {
	return &Loader{}
}

// LoadFromFile loads a recipe from a file path.
func (l *Loader) LoadFromFile(path string) (*ThoughtRecipe, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read recipe file: %w", err)
	}

	return l.LoadFromBytes(data)
}

// LoadFromBytes loads a recipe from a byte slice.
func (l *Loader) LoadFromBytes(data []byte) (*ThoughtRecipe, error) {
	var recipe ThoughtRecipe
	if err := yaml.Unmarshal(data, &recipe); err != nil {
		return nil, fmt.Errorf("failed to unmarshal recipe YAML: %w", err)
	}

	// Validate the recipe
	if err := recipe.Validate(); err != nil {
		return nil, fmt.Errorf("recipe validation failed: %w", err)
	}

	return &recipe, nil
}
