package recipetemplates

import (
	"fmt"

	recipepkg "codeburg.org/lexbit/relurpify/named/euclo/recipes"
)

// LoadAll loads all embedded YAML recipe templates into a RecipeRegistry.
func LoadAll() (*recipepkg.RecipeRegistry, error) {
	registry := recipepkg.NewRecipeRegistry()
	loader := recipepkg.NewLoader()

	// Read all YAML files from the embedded filesystem
	entries, err := templateFS.ReadDir(".")
	if err != nil {
		return registry, fmt.Errorf("failed to read template directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process .yaml files
		if len(entry.Name()) < 5 || entry.Name()[len(entry.Name())-5:] != ".yaml" {
			continue
		}

		// Read file contents
		data, err := templateFS.ReadFile(entry.Name())
		if err != nil {
			return registry, fmt.Errorf("failed to read template file %s: %w", entry.Name(), err)
		}

		// Parse recipe from YAML
		recipe, err := loader.LoadFromBytes(data)
		if err != nil {
			return registry, fmt.Errorf("failed to parse template file %s: %w", entry.Name(), err)
		}

		// Register recipe
		if err := registry.Register(recipe); err != nil {
			return registry, fmt.Errorf("failed to register recipe %s: %w", recipe.ID, err)
		}
	}

	return registry, nil
}
